"""Compare an intended set with the playlist and audio on a mounted USB."""

from __future__ import annotations

import hashlib
import os
import stat
from collections import defaultdict, deque
from dataclasses import dataclass
from pathlib import Path, PureWindowsPath
from urllib.parse import urlsplit
from urllib.request import url2pathname

import legacy_pdb
import one_library
from volume import inspect_volume


DECKS = {
    'CDJ-3000X': 'OneLibrary',
    'CDJ-3000': 'Device Library',
    'CDJ-3000 (firmware 3.30)': 'OneLibrary',
    'CDJ-2000NXS2': 'Device Library',
    'XDJ-XZ': 'Device Library',
    'XDJ-RX3': 'Device Library',
    'XDJ-AZ': 'OneLibrary',
    'OPUS-QUAD': 'OneLibrary',
    'OMNIS-DUO': 'OneLibrary',
}
DATABASE = {'Device Library': 'export.pdb', 'OneLibrary': 'exportLibrary.db'}


@dataclass
class Report:
    status: str
    deck: str
    playlist: str
    evidence: str
    lines: list[str]

    def render(self) -> str:
        return '\n'.join([f'{self.status} | {self.playlist}',
                          f'{self.deck} · {DECKS[self.deck]}', self.evidence, '', *self.lines])


@dataclass
class Track:
    name: str
    path: Path
    digest: bytes | None = None
    size: int | None = None


def read_m3u(path: Path) -> list[str]:
    if path.suffix.lower() not in {'.m3u', '.m3u8'}:
        raise ValueError(f'{path.name}: choose a UTF-8 .m3u8 or .m3u playlist.')
    tracks = []
    with path.open(encoding='utf-8-sig') as stream:
        for line in stream:
            entry = line.rstrip('\r\n')
            if entry.strip() and not entry.startswith('#'):
                tracks.append(entry)
    return tracks


def local_path(entry: str) -> Path:
    if entry.lower().startswith('file:'):
        url = urlsplit(entry)
        if url.netloc.lower() not in {'', 'localhost'} or url.query or url.fragment:
            raise ValueError(f'Unsupported file URL: {entry}')
        return Path(url2pathname(url.path))
    if '://' in entry:
        raise ValueError(f'This check needs a local audio file: {entry}')
    if os.name != 'nt' and PureWindowsPath(entry).drive:
        raise ValueError(f'This playlist points to a Windows drive: {entry}')
    return Path(entry)


def within_drive(usb: Path, path: Path) -> Path:
    resolved = path.resolve()
    if not resolved.is_relative_to(usb):
        raise ValueError(f'The selected USB does not contain this path: {path}')
    return resolved


def device_path(usb: Path, entry: str) -> Path:
    # A database record uses a device-relative path, including the leading slash.
    portable = entry.replace('\\', '/')
    if not portable or '..' in portable.split('/') or PureWindowsPath(portable).drive or '://' in portable:
        raise ValueError(f'Invalid device track path: {entry!r}')
    return within_drive(usb, usb / portable.lstrip('/'))


def snapshot_path(usb: Path, snapshot: Path, entry: str) -> Path:
    path = local_path(entry)
    if path.is_absolute():
        if path.is_relative_to(usb):
            return within_drive(usb, path)
        if path.parts[1:2] == ('Contents',):
            return device_path(usb, str(path))
        raise ValueError(f'The snapshot points off the selected USB: {entry}')
    # Relative paths in an M3U are relative to the playlist file itself.
    return within_drive(usb, snapshot.parent / path)


def fingerprint(path: Path, cache: dict) -> tuple[int, bytes]:
    before = path.stat()
    if not stat.S_ISREG(before.st_mode) or not before.st_size:
        raise ValueError('empty or not a regular audio file')
    key = (path, before.st_size, before.st_mtime_ns, before.st_ino)
    if key not in cache:
        digest = hashlib.sha256()
        with path.open('rb') as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b''):
                digest.update(chunk)
            after = os.fstat(stream.fileno())
        if (before.st_size, before.st_mtime_ns, before.st_ino) != (after.st_size, after.st_mtime_ns, after.st_ino):
            raise ValueError('file changed during the check; run it again')
        cache[key] = digest.digest()
    return before.st_size, cache[key]


def identify(tracks: list[Track], prefix: str, cache: dict) -> list[str]:
    problems = []
    for pos, track in enumerate(tracks, 1):
        try:
            track.size, track.digest = fingerprint(track.path, cache)
        except (OSError, ValueError) as exc:
            problems.append(f'{prefix} track {pos}, {track.name}: {exc}')
    return problems


def find_other_copies(usb: Path, missing: list[Track], cache: dict) -> tuple[set[bytes], bool]:
    """Look for identical audio outside the playlist, hashing only candidate sizes."""
    sizes = {track.size for track in missing}
    wanted = {track.digest for track in missing}
    found = set()
    complete = True

    def skip_unreadable(_error):
        nonlocal complete
        complete = False

    for parent, folders, names in os.walk(usb, onerror=skip_unreadable):
        folders[:] = [f for f in folders if not (Path(parent) / f).is_symlink()]
        for name in names:
            candidate = Path(parent) / name
            try:
                if candidate.is_symlink() or candidate.stat().st_size not in sizes:
                    continue
                if not candidate.is_file():
                    continue
                _, digest = fingerprint(candidate, cache)
                if digest in wanted:
                    found.add(digest)
                    if found == wanted:
                        return found, complete
            except (OSError, ValueError):
                complete = False
    return found, complete


def library_version(path: Path) -> tuple[int, int, int, int]:
    info = path.stat()
    return info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns


def check(usb: Path, source: Path, deck: str, playlist: str | None = None,
          snapshot: Path | None = None) -> Report:
    usb = Path(usb).resolve(strict=True)
    source = Path(source).resolve(strict=True)
    if not usb.is_dir() or deck not in DECKS:
        raise ValueError('Choose a mounted USB folder and a supported deck model.')
    expected = []
    for entry in read_m3u(source):
        path = source.parent / local_path(entry)
        expected.append(Track(path.name, path))
    if not expected:
        raise ValueError('The intended playlist has no tracks. Choose a populated set.')
    name = playlist or source.stem
    library = DECKS[deck]
    db = within_drive(usb, usb / 'PIONEER' / 'rekordbox' / DATABASE[library])
    evidence = f'USB library: {db.name}'
    compatible, volume_note = inspect_volume(usb, deck)

    def report(status, *lines):
        if compatible is False:
            status = 'DIFFERENCES'
        elif compatible is None and status == 'MATCH':
            status = 'INCOMPLETE'
        return Report(status, deck, name, evidence, [*lines, '', volume_note])

    if not db.is_file():
        return report('DIFFERENCES', f'The USB is missing {library} ({db.relative_to(usb)}).',
                      'Select the root of the USB. If this is the right drive, export the set to it from rekordbox.')
    version = library_version(db)
    try:
        reader = legacy_pdb if library == 'Device Library' else one_library
        playlists = reader.read_playlists(db)
    except (ValueError, OSError) as exc:
        if snapshot is None:
            return report('INCOMPLETE', f'Could not read {library}: {exc}',
                          f'In rekordbox open Devices → this USB → {library} → Playlists. '
                          'Export the device playlist to M3U8 and select it as the snapshot.')
        snapshot = Path(snapshot).resolve(strict=True)
        if source.samefile(snapshot):
            return report('INCOMPLETE', 'Choose a separate playlist exported from the USB library.')
        evidence = f'Device playlist snapshot supplied by you: {snapshot.name}'
        actual = []
        for entry in read_m3u(snapshot):
            path = snapshot_path(usb, snapshot, entry)
            actual.append(Track(path.name, path))
        direct = False
    else:
        matches = [entries for label, entries in playlists if label == name]
        if not matches:
            return report('DIFFERENCES', f'Playlist “{name}” is missing from {library}.',
                          'Check the exact playlist name or export it to this USB in rekordbox.')
        if len(matches) > 1:
            return report('INCOMPLETE', f'{len(matches)} playlists are named “{name}”. '
                          'Give the set a unique name and export it again.')
        actual = []
        for entry, title in matches[0]:
            try:
                path = device_path(usb, entry)
            except (OSError, ValueError) as exc:
                return report('INCOMPLETE', f'Invalid USB library reference for {title}: {exc}')
            actual.append(Track(title or path.name, path))
        direct = True

    cache = {}
    source_errors = identify(expected, 'Intended', cache)
    actual_errors = identify(actual, 'USB', cache)
    lines = [f'{len(expected)} intended tracks · {len(actual)} exported entries', *source_errors, *actual_errors]
    if source_errors or actual_errors:
        lines.append('Reconnect the source audio and re-export any missing or empty USB files, then check again.')
        return report('INCOMPLETE', *lines)

    # Match each occurrence independently, including repeated tracks in the same set.
    positions = defaultdict(deque)
    for pos, track in enumerate(actual, 1):
        positions[track.digest].append(pos)
    matched = []
    missing = []
    for pos, track in enumerate(expected, 1):
        if positions[track.digest]:
            matched.append((pos, positions[track.digest].popleft(), track.name))
        else:
            missing.append((pos, track))
    extras = sorted(pos for remaining in positions.values() for pos in remaining)
    if missing:
        copies, scan_complete = find_other_copies(usb, [track for _, track in missing], cache)
        if not scan_complete:
            lines.append('Some USB files could not be searched for matching audio.')
        for pos, track in missing:
            detail = ('identical audio found on USB' if track.digest in copies else
                      'no identical file found in the readable USB files')
            lines.append(f'Missing from playlist at {pos}: {track.name} ({detail}).')
    for pos in extras:
        lines.append(f'Unexpected USB track at {pos}: {actual[pos - 1].name}.')
    out_of_order = [dest for _, dest, _ in matched] != sorted(dest for _, dest, _ in matched)
    if out_of_order:
        for pos, dest, title in matched:
            if pos != dest:
                lines.append(f'Order: {title} is at {dest} on USB, intended at {pos}.')
    if missing or extras or out_of_order:
        lines.append('Reconcile the playlist in rekordbox, export it again, and recheck.')
        return report('DIFFERENCES', *lines)
    if library_version(db) != version:
        return report('INCOMPLETE', 'The USB library changed during the check. Finish exporting and run it again.')
    lines.append('Every playlist entry matches the intended audio byte for byte, in order.')
    if not direct:
        lines.append('Order is from the supplied snapshot. The live device library could not be read.')
        return report('INCOMPLETE', *lines)
    return report('MATCH', *lines)
