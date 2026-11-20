import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from volume import inspect_volume


class VolumeTests(unittest.TestCase):
    def test_format_depends_on_deck_and_firmware(self):
        cases = [
            ("MS-DOS FAT32", "CDJ-2000NXS2", True),
            ("exFAT", "CDJ-2000NXS2", False),
            ("exFAT", "CDJ-3000X", True),
            ("exFAT", "CDJ-3000", None),
            ("exFAT", "CDJ-3000 (firmware 3.30)", True),
            ("NTFS", "XDJ-AZ", False),
            ("vfat", "XDJ-XZ", None),
            ("", "XDJ-RX3", None),
        ]
        with tempfile.TemporaryDirectory() as root:
            for detected, deck, expected in cases:
                with self.subTest(detected=detected, deck=deck):
                    with patch("volume._filesystem", return_value=detected):
                        supported, message = inspect_volume(Path(root), deck)
                    self.assertIs(supported, expected)
                    self.assertTrue(message)
