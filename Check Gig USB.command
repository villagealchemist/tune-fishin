#!/bin/sh
cd "$(dirname "$0")" || exit 1
python3 gig_usb/tuna.py
status=$?
if [ "$status" -ne 0 ]; then
  echo "Use Python 3.10 or newer with Tkinter to open the Tune Fishin window."
  printf 'Press Return to close. '
  read -r reply
fi
exit "$status"
