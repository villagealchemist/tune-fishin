"""Launch Tune Fishin's gig USB checker, or run one check in the terminal."""

import argparse
import sys
from pathlib import Path

from preflight import DECKS, check


def main(argv=None):
    parser = argparse.ArgumentParser(description="Read back a rekordbox playlist from a mounted gig USB.")
    subcommands = parser.add_subparsers(dest="command")
    checker = subcommands.add_parser("check", help="compare the set with the device export")
    checker.add_argument("--usb", type=Path, required=True, help="mounted USB root")
    checker.add_argument("--set", dest="source", type=Path, required=True, help="intended local UTF-8 M3U playlist")
    checker.add_argument("--deck", required=True, choices=sorted(DECKS))
    checker.add_argument("--playlist", help="device playlist name (defaults to the intended filename stem)")
    checker.add_argument("--snapshot", type=Path, help="fresh M3U exported from the device playlist in rekordbox")
    args = parser.parse_args(argv)
    if not args.command:
        try:
            from gui import launch
        except ImportError as exc:
            print(f"The window requires Python with Tkinter ({exc}). The check command also works in a terminal.",
                  file=sys.stderr)
            return 2
        launch()
        return 0
    try:
        report = check(args.usb, args.source, args.deck, args.playlist, args.snapshot)
    except (OSError, ValueError) as exc:
        print(f"INCOMPLETE | {exc}", file=sys.stderr)
        return 2
    print(report.render())
    return {"MATCH": 0, "DIFFERENCES": 1, "INCOMPLETE": 2}[report.status]


if __name__ == "__main__":
    sys.exit(main())
