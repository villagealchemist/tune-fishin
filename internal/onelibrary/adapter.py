"""Print ordered playlist rows from the private OneLibrary snapshot in the current directory."""

import json
import sys

if sys.version_info < (3, 11):
    sys.exit(20)

try:
    from pyrekordbox.onelibrary import OneLibrary
    from sqlalchemy.exc import SQLAlchemyError
except ImportError:
    sys.exit(20)


def emit(record):
    print(json.dumps(record, ensure_ascii=True, separators=(",", ":")))


def main():
    library = OneLibrary(path="exportLibrary.db")
    try:
        with library.engine.connect() as connection:
            connection.exec_driver_sql("PRAGMA query_only = ON")
            rows = connection.exec_driver_sql("""
                SELECT p.playlist_id, p.name, pc.playlist_id, pc.sequenceNo, c.path, c.title
                FROM playlist AS p
                LEFT JOIN playlist_content AS pc ON pc.playlist_id = p.playlist_id
                LEFT JOIN content AS c ON c.content_id = pc.content_id
                WHERE p.attribute = 0
                ORDER BY p.playlist_id, pc.sequenceNo
            """)
            previous_id = None
            for playlist_id, name, member_id, position, path, title in rows:
                if playlist_id != previous_id:
                    emit({"name": name})
                    previous_id = playlist_id
                if member_id is not None:
                    emit({"position": position, "path": path, "title": title})
            emit({"done": True})
    finally:
        library.close()
        library.engine.dispose()


try:
    main()
except ImportError:
    sys.exit(20)
except SQLAlchemyError:
    sys.exit(21)
