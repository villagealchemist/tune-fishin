import struct
import unittest
from pathlib import Path
from tempfile import TemporaryDirectory

from legacy_pdb import read_playlists


def short_string(text):
    encoded = text.encode("ascii")
    return bytes([2 * (len(encoded) + 1) + 1]) + encoded


def track(track_id, title):
    row = bytearray(136)
    struct.pack_into("<H", row, 0, 0x24)
    struct.pack_into("<I", row, 72, track_id)
    for column, value in [(17, title), (20, f"/Contents/{title}.mp3")]:
        struct.pack_into("<H", row, 94 + column * 2, len(row))
        row.extend(short_string(value))
    return row


def export(entries):
    page_size = 512
    tree = struct.pack("<5I", 0, 0, 1, 7, 0) + short_string("Friday")
    tables = [
        (0, [track(11, "Intro"), None, track(22, "Break")]),
        (7, [tree]),
        (8, [struct.pack("<III", position, track_id, 7) for position, track_id in entries]),
    ]
    header = bytearray(page_size)
    struct.pack_into("<III", header, 0, 0, page_size, len(tables))
    pages = [header]
    for index, (table, rows) in enumerate(tables, 1):
        struct.pack_into("<4I", header, 28 + (index - 1) * 16, table, 0, index, index)
        page = bytearray(page_size)
        struct.pack_into("<II", page, 4, index, table)
        page[27] = 0x24
        present = 0
        cursor = 40
        for slot, row in enumerate(rows):
            if row is None:
                continue
            present |= 1 << slot
            struct.pack_into("<H", page, page_size - 6 - 2 * slot, cursor - 40)
            page[cursor:cursor + len(row)] = row
            cursor += len(row)
        counts = len(rows) | (bin(present).count("1") << 13)
        page[24:27] = counts.to_bytes(3, "little")
        struct.pack_into("<H", page, page_size - 4, present)
        pages.append(page)
    return b"".join(pages)


class LegacyPdbTests(unittest.TestCase):
    def read_export(self, entries):
        with TemporaryDirectory() as directory:
            path = Path(directory) / "export.pdb"
            path.write_bytes(export(entries))
            return read_playlists(path)

    def test_order_and_repeated_tracks_survive_deleted_slot(self):
        intro = ("/Contents/Intro.mp3", "Intro")
        middle = ("/Contents/Break.mp3", "Break")
        playlists = self.read_export([(2, 22), (3, 11), (1, 11)])
        self.assertEqual(playlists, [("Friday", [intro, middle, intro])])

    def test_dangling_track_reference_is_an_error(self):
        with self.assertRaisesRegex(ValueError, "missing track ID 999"):
            self.read_export([(1, 999)])

    def test_duplicate_positions_are_an_error(self):
        with self.assertRaisesRegex(ValueError, "same position"):
            self.read_export([(1, 11), (1, 22)])


if __name__ == "__main__":
    unittest.main()
