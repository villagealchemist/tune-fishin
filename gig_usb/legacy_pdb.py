"""Read playlist order and audio paths from a rekordbox Device Library export.

Format reference: https://djl-analysis.deepsymmetry.org/rekordbox-export-analysis/exports.html
"""

from __future__ import annotations

import os
import struct
from collections import defaultdict
from pathlib import Path
from typing import BinaryIO, Iterator

_TRACKS = 0
_PLAYLISTS = 7
_ENTRIES = 8
_PAGE_HEADER = 40
_TRACK_OFFSETS = 94


def _u16(data: bytes, offset: int) -> int:
    return struct.unpack_from("<H", data, offset)[0]


def _string(row: bytes, offset: int) -> str:
    if offset >= len(row):
        raise ValueError("A Device Library string points past its row.")
    kind = row[offset]
    if kind & 1:
        size = kind >> 1
        header, encoding = 1, "ascii"
    elif kind in (0x40, 0x90) and offset + 4 <= len(row):
        size = _u16(row, offset + 1)
        header = 4
        encoding = "ascii" if kind == 0x40 else "utf-16-le"
    else:
        raise ValueError(f"Unsupported Device Library string format 0x{kind:02x}.")
    if size < header or offset + size > len(row):
        raise ValueError("A Device Library string has an invalid length.")
    return row[offset + header : offset + size].decode(encoding)


def _rows(page: bytes) -> Iterator[bytes]:
    # The 13-bit allocated slot count includes deleted rows. Only the
    # presence mask in each group of sixteen slots authorizes reading a row.
    counts = int.from_bytes(page[24:27], "little")
    slots = counts & 0x1FFF
    heap_end = len(page) - (slots * 2 + ((slots + 15) // 16) * 4)
    if heap_end < _PAGE_HEADER:
        raise ValueError("The Device Library row index overlaps its page header.")
    offsets = []
    for group in range((slots + 15) // 16):
        end = len(page) - group * 36
        present = _u16(page, end - 4)
        for slot in range(min(16, slots - group * 16)):
            if present & (1 << slot):
                offsets.append(_PAGE_HEADER + _u16(page, end - 6 - slot * 2))
    if len(offsets) != counts >> 13:
        raise ValueError("The Device Library row count disagrees with its index.")
    offsets.sort()
    for start, stop in zip(offsets, offsets[1:] + [heap_end]):
        if start >= stop or stop > heap_end:
            raise ValueError("The Device Library contains overlapping or invalid rows.")
        yield page[start:stop]


def _table_rows(stream: BinaryIO, page_size: int, page_count: int, table: int,
                first: int, last: int) -> Iterator[bytes]:
    seen = set()
    index = first
    while True:
        if not 0 < index < page_count or index in seen:
            raise ValueError("The Device Library contains a broken page chain.")
        seen.add(index)
        stream.seek(index * page_size)
        page = stream.read(page_size)
        if len(page) != page_size:
            raise ValueError("The Device Library ended during a read.")
        stored_index, stored_table, following = struct.unpack_from("<III", page, 4)
        if (stored_index, stored_table) != (index, table):
            raise ValueError("The Device Library contains a mismatched page pointer.")
        if not page[27] & 0x40:
            yield from _rows(page)
        if index == last:
            break
        index = following


def read_playlists(path: Path) -> list[tuple[str, list[tuple[str, str]]]]:
    """Return playlist names with ordered (device audio path, track title) pairs.

    Preserve duplicate names across folders and repeated tracks within a set.
    Raise ValueError for an unsupported or inconsistent library and OSError for
    a filesystem failure.
    """
    tracks = {}
    playlists = {}
    entries = defaultdict(dict)
    with path.open("rb") as stream:
        header = stream.read(28)
        if len(header) != 28:
            raise ValueError("The Device Library header is incomplete.")
        signature, page_size, table_count = struct.unpack_from("<III", header)
        size = os.fstat(stream.fileno()).st_size
        if (signature or not 512 <= page_size <= 65536 or size % page_size
                or size < page_size or 28 + table_count * 16 > page_size):
            raise ValueError("Unsupported or incomplete Device Library header.")
        pointers = stream.read(table_count * 16)
        if len(pointers) != table_count * 16:
            raise ValueError("The Device Library table list is incomplete.")
        seen_tables = set()
        for table, _, first, last in struct.iter_unpack("<IIII", pointers):
            if table not in (_TRACKS, _PLAYLISTS, _ENTRIES):
                continue
            if table in seen_tables:
                raise ValueError("The Device Library has duplicate table pointers.")
            seen_tables.add(table)
            for row in _table_rows(stream, page_size, size // page_size, table, first, last):
                if table == _TRACKS:
                    if len(row) < _TRACK_OFFSETS + 42 or _u16(row, 0) != 0x24:
                        raise ValueError("Unsupported Device Library track layout.")
                    track_id = struct.unpack_from("<I", row, 72)[0]
                    title_at = _u16(row, _TRACK_OFFSETS + 2 * 17)
                    path_at = _u16(row, _TRACK_OFFSETS + 2 * 20)
                    if min(title_at, path_at) < _TRACK_OFFSETS + 42 or track_id in tracks:
                        raise ValueError("Invalid or duplicated Device Library track.")
                    tracks[track_id] = (_string(row, path_at), _string(row, title_at))
                elif table == _PLAYLISTS:
                    if len(row) < 21:
                        raise ValueError("A Device Library playlist row is incomplete.")
                    playlist_id, folder = struct.unpack_from("<II", row, 12)
                    if not folder:
                        if playlist_id in playlists:
                            raise ValueError("The Device Library repeats a playlist ID.")
                        playlists[playlist_id] = _string(row, 20)
                else:
                    if len(row) < 12:
                        raise ValueError("A Device Library playlist entry is incomplete.")
                    position, track_id, playlist_id = struct.unpack_from("<III", row)
                    if position in entries[playlist_id]:
                        raise ValueError("Two Device Library entries have the same position.")
                    entries[playlist_id][position] = track_id

    if seen_tables != {_TRACKS, _PLAYLISTS, _ENTRIES}:
        raise ValueError("The Device Library is missing a required playlist or track table.")
    if entries.keys() - playlists.keys():
        raise ValueError("A Device Library entry refers to an absent playlist.")
    result = []
    for playlist_id, name in playlists.items():
        ordered = []
        for _, track_id in sorted(entries[playlist_id].items()):
            if track_id not in tracks:
                raise ValueError(f"Playlist {name!r} refers to missing track ID {track_id}.")
            ordered.append(tracks[track_id])
        result.append((name, ordered))
    return result
