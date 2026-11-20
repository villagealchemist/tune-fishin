"""Exercises the optional OneLibrary reader against an encrypted fixture it creates."""

import hashlib
import tempfile
import unittest
from pathlib import Path

from one_library import read_playlists


class OneLibraryTest(unittest.TestCase):
    def setUp(self):
        folder = tempfile.TemporaryDirectory()
        self.addCleanup(folder.cleanup)
        database_dir = Path(folder.name) / "PIONEER" / "rekordbox"
        database_dir.mkdir(parents=True)
        self.path = database_dir / "exportLibrary.db"

    def test_pending_journal_is_unverified_without_optional_dependencies(self):
        self.path.write_bytes(b"placeholder")
        self.path.with_name("exportLibrary.db-wal").write_bytes(b"pending")
        with self.assertRaisesRegex(ValueError, "journal"):
            read_playlists(self.path)

    def create_library(self):
        try:
            from pyrekordbox.onelibrary import OneLibrary
            from sqlcipher3 import dbapi2  # noqa: F401
        except ImportError:
            self.skipTest("Install the optional OneLibrary reader to exercise encrypted exports.")

        self.path.touch()
        library = OneLibrary(path=self.path)
        try:
            with library.engine.begin() as connection:
                for statement in (
                    "CREATE TABLE playlist (playlist_id INTEGER PRIMARY KEY, name TEXT, attribute INTEGER)",
                    "CREATE TABLE playlist_content (playlist_id INTEGER, content_id INTEGER, sequenceNo INTEGER)",
                    "CREATE TABLE content (content_id INTEGER PRIMARY KEY, path TEXT, title TEXT)",
                    "INSERT INTO playlist VALUES (1,'Set',0),(2,'Empty',0),(3,'Folder',1),(4,'Set',0)",
                    "INSERT INTO playlist_content VALUES (1,20,3),(1,10,1),(1,20,2)",
                    "INSERT INTO content VALUES (10,'/Contents/open.aif','Opener'),(20,'/Contents/end.flac','Closer')",
                ):
                    connection.exec_driver_sql(statement)
        finally:
            library.close()
            library.engine.dispose()

    def test_encrypted_playlist_order_and_source_preservation(self):
        self.create_library()
        self.assertFalse(self.path.read_bytes().startswith(b"SQLite format"))
        source_hash = hashlib.sha256(self.path.read_bytes()).digest()
        modified = self.path.stat().st_mtime_ns
        files = set(self.path.parent.iterdir())
        opener = ("/Contents/open.aif", "Opener")
        closer = ("/Contents/end.flac", "Closer")
        self.assertEqual(
            read_playlists(self.path),
            [("Set", [opener, closer, closer]), ("Empty", []), ("Set", [])],
        )
        self.assertEqual(set(self.path.parent.iterdir()), files)
        self.assertEqual(hashlib.sha256(self.path.read_bytes()).digest(), source_hash)
        self.assertEqual(self.path.stat().st_mtime_ns, modified)
