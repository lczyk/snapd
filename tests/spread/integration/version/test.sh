#!/usr/bin/env bash
# snap version output format parity with the real snap CLI.
# checks that all six fields are present and the series is always 16.

set -eux

OUT="$(snap version)"

echo "$OUT" | grep -q "^snap "
echo "$OUT" | grep -q "^snapd "
echo "$OUT" | grep -q "^series "
echo "$OUT" | grep -q "^os "
echo "$OUT" | grep -q "^kernel "
echo "$OUT" | grep -q "^architecture "

# series is always 16 in the snap ecosystem
echo "$OUT" | grep -q "^series *16$"

# kernel should match uname -r
KERNEL="$(uname -r)"
echo "$OUT" | grep -q "^kernel *${KERNEL}$"

# architecture should match dpkg / uname
ARCH="$(dpkg --print-architecture 2>/dev/null || uname -m)"
# normalise: x86_64 -> amd64, aarch64 -> arm64 (snap uses debian names)
case "$ARCH" in
    x86_64)  ARCH=amd64 ;;
    aarch64) ARCH=arm64 ;;
esac
echo "$OUT" | grep -q "^architecture *${ARCH}$"
