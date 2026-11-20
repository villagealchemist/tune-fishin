import io
import unittest
import wave
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest.mock import patch

from preflight import check


def audio(sample):
    buffer = io.BytesIO()
    with wave.open(buffer, "wb") as stream:
        stream.setparams((1, 1, 8000, 0, "NONE", "not compressed"))
        stream.writeframes(bytes([sample]) * 160)
    return buffer.getvalue()


class GigUSBCheckTests(unittest.TestCase):
    def setUp(self):
        volume = patch("preflight.inspect_volume", return_value=(True, "Test FAT32 volume"))
        self.volume = volume.start()
        self.addCleanup(volume.stop)
        directory = TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.usb = self.root / "Gig USB"
        catalog = self.usb / "PIONEER" / "rekordbox" / "export.pdb"
        catalog.parent.mkdir(parents=True)
        catalog.touch()
        self.files = self.usb / "Contents"
        self.files.mkdir()

        collection = self.root / "Studio collection"
        collection.mkdir()
        self.opener = collection / "Opener.wav"
        self.closer = collection / "Closer.wav"
        self.opener.write_bytes(audio(20))
        self.closer.write_bytes(audio(40))
        for track in [self.opener, self.closer]:
            (self.files / track.name).write_bytes(track.read_bytes())
        self.setlist = self.root / "Friday.m3u8"
        self.make_set([self.opener, self.closer])

    def make_set(self, tracks):
        self.setlist.write_text(
            "#EXTM3U\n" + "".join(f"{track}\n" for track in tracks),
            encoding="utf-8",
        )

    def inspect(self, tracks, name="Friday"):
        entries = [(f"/Contents/{track.name}", track.stem) for track in tracks]
        with patch("preflight.legacy_pdb.read_playlists", return_value=[(name, entries)]):
            return check(self.usb, self.setlist, "CDJ-2000NXS2", playlist="Friday")

    def test_matching_audio_in_matching_order(self):
        report = self.inspect([self.opener, self.closer])
        self.assertEqual(report.status, "MATCH", report.render())

    def test_unknown_volume_cannot_confirm_ready(self):
        self.volume.return_value = None, "Unknown filesystem"
        report = self.inspect([self.opener, self.closer])
        self.assertEqual(report.status, "INCOMPLETE", report.render())

    def test_incompatible_filesystem_with_matching_playlist(self):
        self.volume.return_value = False, "This deck cannot read NTFS."
        report = self.inspect([self.opener, self.closer])
        self.assertEqual(report.status, "DIFFERENCES", report.render())

    def test_reversed_order(self):
        report = self.inspect([self.closer, self.opener])
        self.assertEqual(report.status, "DIFFERENCES", report.render())

    def test_missing_second_occurrence_of_a_track(self):
        self.make_set([self.opener, self.closer, self.opener])
        report = self.inspect([self.opener, self.closer])
        self.assertEqual(report.status, "DIFFERENCES", report.render())

    def test_named_playlist_missing_but_audio_present(self):
        report = self.inspect([self.opener, self.closer], name="Last week's set")
        self.assertNotEqual(report.status, "MATCH", report.render())

    def test_same_filename_and_size_with_different_audio(self):
        (self.files / self.closer.name).write_bytes(audio(80))
        report = self.inspect([self.opener, self.closer])
        self.assertEqual(report.status, "DIFFERENCES", report.render())

    def test_catalog_entry_points_to_missing_audio(self):
        (self.files / self.closer.name).unlink()
        report = self.inspect([self.opener, self.closer])
        self.assertNotEqual(report.status, "MATCH", report.render())

    def test_audio_symlink_to_laptop_is_not_present_on_usb(self):
        link = self.files / self.closer.name
        link.unlink()
        try:
            link.symlink_to(self.closer)
        except OSError as error:
            self.skipTest(f"This system cannot create a symlink: {error}")
        report = self.inspect([self.opener, self.closer])
        self.assertNotEqual(report.status, "MATCH", report.render())

    def test_source_audio_offline(self):
        self.closer.unlink()
        report = self.inspect([self.opener, self.closer])
        self.assertNotEqual(report.status, "MATCH", report.render())

    def inspect_snapshot(self, snapshot):
        with patch("preflight.legacy_pdb.read_playlists", side_effect=ValueError("Unreadable library")):
            return check(self.usb, self.setlist, "CDJ-2000NXS2", snapshot=snapshot)

    def test_matching_user_snapshot_cannot_certify_the_live_library(self):
        snapshot = self.root / "device playlist.m3u8"
        snapshot.write_text("/Contents/Opener.wav\n/Contents/Closer.wav\n", encoding="utf-8")
        report = self.inspect_snapshot(snapshot)
        self.assertEqual(report.status, "INCOMPLETE", report.render())
        self.assertIn("snapshot", report.render().lower())

    def test_snapshot_can_identify_reordered_tracks(self):
        snapshot = self.root / "device playlist.m3u8"
        snapshot.write_text("/Contents/Closer.wav\n/Contents/Opener.wav\n", encoding="utf-8")
        report = self.inspect_snapshot(snapshot)
        self.assertEqual(report.status, "DIFFERENCES", report.render())
        self.assertIn("snapshot", report.render().lower())

    def test_source_playlist_cannot_double_as_device_snapshot(self):
        report = self.inspect_snapshot(self.setlist)
        self.assertEqual(report.status, "INCOMPLETE", report.render())

    def test_live_library_overrides_a_conflicting_snapshot(self):
        snapshot = self.root / "old device playlist.m3u8"
        snapshot.write_text("/Contents/Opener.wav\n/Contents/Closer.wav\n", encoding="utf-8")
        live_playlists = [("Friday", [("/Contents/Closer.wav", "Closer")])]
        with patch("preflight.legacy_pdb.read_playlists", return_value=live_playlists):
            report = check(self.usb, self.setlist, "CDJ-2000NXS2", snapshot=snapshot)
        self.assertEqual(report.status, "DIFFERENCES", report.render())


if __name__ == "__main__":
    unittest.main()
