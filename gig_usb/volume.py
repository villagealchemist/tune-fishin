"""Read-only checks of a mounted volume's filesystem, scoped to USB playback."""

from __future__ import annotations

import ctypes
import json
import os
from pathlib import Path
import plistlib
import subprocess
import sys


# https://cdn.rekordbox.com/files/20260318114024/OneLibrary-Compatible-USB-Device-Export_en.pdf
_MODERN = {"CDJ-3000X", "XDJ-AZ", "OPUS-QUAD", "OMNIS-DUO"}
# https://rekordbox.com/en/2022/03/rekordbox-v662-release-information/
_EXFAT_FIRMWARE = {"CDJ-3000": "1.20", "XDJ-XZ": "1.23", "XDJ-RX3": "1.11"}
_KNOWN_DECKS = _MODERN | set(_EXFAT_FIRMWARE) | {"CDJ-2000NXS2"}


def _run(*args: str) -> bytes:
    return subprocess.run(args, capture_output=True, check=True, timeout=5).stdout


def _filesystem(root: Path) -> str:
    if sys.platform == "darwin":
        info = plistlib.loads(_run("/usr/sbin/diskutil", "info", "-plist", str(root)))
        if info.get("MountPoint") != str(root):
            return ""
        # FilesystemName distinguishes MS-DOS FAT variants; FilesystemType is just msdos.
        return info.get("FilesystemName") or info.get("FilesystemType") or ""

    if sys.platform.startswith("linux"):
        data = json.loads(_run("findmnt", "--json", "--mountpoint", str(root),
                               "--output", "SOURCE,FSTYPE,FSROOT"))
        volumes = data.get("filesystems", [])
        if len(volumes) != 1:
            return ""
        volume = volumes[0]
        if volume.get("fsroot") != "/" or not str(volume.get("source", "")).startswith("/dev/"):
            return ""
        fs = volume.get("fstype") or ""
        if fs in {"vfat", "msdos"}:
            # A generic FAT mount may even be FAT12. lsblk can expose the FAT variant.
            try:
                version = _run("lsblk", "--nodeps", "--noheadings", "--output",
                               "FSVER", volume["source"]).decode().strip()
            except (OSError, subprocess.SubprocessError):
                return fs
            if version in {"FAT12", "FAT16", "FAT32"}:
                return version
        return fs

    if sys.platform == "win32" and root.drive and root == Path(root.anchor):
        kernel = ctypes.windll.kernel32
        if kernel.GetDriveTypeW(str(root)) not in (2, 3):
            return ""
        fs = ctypes.create_unicode_buffer(64)
        if kernel.GetVolumeInformationW(str(root), None, 0, None, None, None, fs, len(fs)):
            return fs.value
    return ""


def inspect_volume(usb: Path, deck: str) -> tuple[bool | None, str]:
    """Return compatibility of the filesystem alone; None means it needs confirmation."""
    generic = "Filesystem unverified. Select the mounted USB volume itself to check its format."
    try:
        root = usb.resolve(strict=True)
        if not root.is_dir() or (os.name != "nt" and root == Path("/")):
            return None, generic
        raw = _filesystem(root)
    except (OSError, ValueError, subprocess.SubprocessError, plistlib.InvalidFileException):
        return None, generic
    if not raw:
        return None, generic

    if deck == "CDJ-3000 (firmware 3.30)":
        if raw.lower().strip() == "exfat":
            return True, "exFAT is supported on CDJ-3000 firmware 3.30."
        deck = "CDJ-3000"
    deck = deck.upper()
    if deck not in _KNOWN_DECKS:
        return None, f"Detected {raw}. Filesystem compatibility is unknown for {deck}."
    label = raw.lower().strip()
    if label in {"fat32", "ms-dos fat32", "fat16", "ms-dos fat16"}:
        return True, f"{raw} is a supported filesystem on the {deck}."
    if label == "exfat":
        if deck in _EXFAT_FIRMWARE:
            version = _EXFAT_FIRMWARE[deck]
            return None, f"exFAT requires {deck} firmware {version} or newer. Check the venue's deck."
        if deck in _MODERN:
            return True, f"exFAT is a supported filesystem on the {deck}."
        return False, f"The {deck} cannot read exFAT."
    if label in {"hfs+", "journaled hfs+", "mac os extended", "mac os extended (journaled)"}:
        return True, f"HFS+ is a supported filesystem on the {deck}."
    if label in {"hfs", "hfsplus", "hfsx"} or "case-sensitive" in label:
        return None, f"Detected {raw}. Confirm the USB uses a compatible HFS+ variant for the {deck}."
    if label in {"ntfs", "ntfs3", "apfs", "ext2", "ext3", "ext4", "xfs", "btrfs",
                 "fat12", "ms-dos fat12", "refs"}:
        return False, f"The {deck} does not support the {raw} filesystem."
    return None, f"Detected {raw}. The exact supported filesystem could not be verified."
