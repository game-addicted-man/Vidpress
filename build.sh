#!/bin/sh
# Cross-compile dist/VidPress/VidPress.exe from Linux or macOS (Go 1.24+).
# usage: ./build.sh [version]
set -e
cd "$(dirname "$0")"
VERSION="${1:-1.0.0}"
mkdir -p dist/VidPress
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.appVersion=$VERSION" \
  -o dist/VidPress/VidPress.exe .
cp docs/*.txt dist/VidPress/
cp LICENSE dist/VidPress/LICENSE.txt
cp THIRD_PARTY_NOTICES.md dist/VidPress/THIRD_PARTY_NOTICES.txt
echo "built dist/VidPress/VidPress.exe ($VERSION)"
if [ ! -f dist/VidPress/ffmpeg/ffmpeg.exe ]; then
  echo "note: put ffmpeg.exe, ffprobe.exe and the DLLs of a BtbN win64-gpl-shared build into dist/VidPress/ffmpeg/"
fi
