#!/bin/sh
cd "$(dirname "$0")" || exit 1
if [ -x ./tuna ]; then
  exec ./tuna
fi
if command -v go >/dev/null 2>&1; then
  exec go run ./cmd/tuna
fi
echo "Place the macOS tuna binary next to this launcher, or install Go 1.23 or later to run from source."
printf 'Press Return to close. '
read -r reply
exit 2
