"""Read a stable, local snapshot of an exported rekordbox OneLibrary database."""

import shutil
import stat
import tempfile
from pathlib import Path, PurePosixPath

_INSTALL = (
    "Direct OneLibrary checks require Python 3.11+ with pyrekordbox 0.5 and its "
    "SQLCipher driver. Install pyrekordbox==0.5.0 and sqlcipher3-wheels in the "
    "same Python environment, or supply a fresh playlist export from the USB."
)

_PLAYLIST_ROWS = """
    SELECT p.playlist_id, p.name, pc.playlist_id, pc.sequenceNo, c.path, c.title
    FROM playlist AS p
    LEFT JOIN playlist_content AS pc ON pc.playlist_id = p.playlist_id
    LEFT JOIN content AS c ON c.content_id = pc.content_id
    WHERE p.attribute = 0
    ORDER BY p.playlist_id, pc.sequenceNo
"""


def _snapshot_state(path):
    """Reject incomplete exports rather than ignoring pending changes in journals."""
    for suffix in ("-wal", "-journal"):
        journal = path.with_name(path.name + suffix)
        try:
            info = journal.lstat()
        except FileNotFoundError:
            continue
        if not stat.S_ISREG(info.st_mode) or info.st_size:
            raise ValueError(
                "OneLibrary has an active journal. Finish exporting, safely eject "
                "and reconnect the drive, then repeat the check."
            )

    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or not info.st_size:
        raise ValueError("The exported OneLibrary database is empty or is not a regular file.")
    return info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns


def _read_rows(rows):
    playlists = []
    previous_id = None
    previous_position = None
    for playlist_id, name, member_id, position, path, title in rows:
        if playlist_id != previous_id:
            if not isinstance(name, str) or not name:
                raise ValueError("OneLibrary contains an unnamed playlist.")
            entries = []
            playlists.append((name, entries))
            previous_id = playlist_id
            previous_position = None
        if member_id is None:
            continue
        if not isinstance(position, int) or position == previous_position:
            raise ValueError(f"OneLibrary has an ambiguous track order in {name!r}.")
        if not isinstance(path, str) or not path or "\0" in path:
            raise ValueError(f"OneLibrary has a missing track reference in {name!r}.")
        previous_position = position
        entries.append((path, title or PurePosixPath(path).name))
    return playlists


def read_playlists(path: Path) -> list:
    """Return ordinary playlists, their device audio paths and titles in set order."""
    path = Path(path)
    state = _snapshot_state(path)
    try:
        from pyrekordbox.onelibrary import OneLibrary
        from sqlalchemy.exc import SQLAlchemyError
    except ImportError:
        raise ValueError(_INSTALL) from None

    # SQLite can create sidecars even for queries. Only the copy reaches the driver.
    temp_root = Path(tempfile.gettempdir()).resolve()
    device_root = path.parent.resolve()
    if device_root.name.lower() == "rekordbox" and device_root.parent.name.lower() == "pioneer":
        device_root = device_root.parent.parent
    if temp_root == device_root or device_root in temp_root.parents:
        raise ValueError("Choose a temporary directory on your computer outside the USB.")

    with tempfile.TemporaryDirectory(prefix="tune-fishin-", dir=temp_root) as directory:
        snapshot = Path(directory) / "exportLibrary.db"
        shutil.copyfile(path, snapshot)
        if _snapshot_state(path) != state or snapshot.stat().st_size != state[2]:
            raise ValueError("OneLibrary changed while it was being copied. Finish exporting and retry.")
        try:
            library = OneLibrary(path=snapshot)
            try:
                with library.engine.connect() as connection:
                    connection.exec_driver_sql("PRAGMA query_only = ON")
                    playlists = _read_rows(connection.exec_driver_sql(_PLAYLIST_ROWS))
            finally:
                library.close()
                library.engine.dispose()
        except ImportError:
            raise ValueError(_INSTALL) from None
        except SQLAlchemyError:
            raise ValueError(
                "The OneLibrary reader could not read this export. Recheck the "
                "on-device playlist in rekordbox or supply a fresh device playlist export."
            ) from None
    if _snapshot_state(path) != state:
        raise ValueError("OneLibrary changed during the check. Finish exporting and retry.")
    return playlists
