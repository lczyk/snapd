#!/usr/bin/env bash
# snap download fetches the .snap + .assert bundle to disk without
# installing. the snap must not appear in `snap list` afterwards.

set -eux

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

# -- basic download to target-directory --

snap download --target-directory="$WORKDIR" hello-world

# .snap file should exist
ls "$WORKDIR"/hello-world_*.snap
# .assert file should exist alongside it
ls "$WORKDIR"/hello-world_*.assert

# not installed
! snap list | grep -q "^hello-world "

# -- output contains install hint --

OUT="$(snap download --target-directory="$WORKDIR" hello-world 2>&1 || true)"
echo "$OUT" | grep -qi "snap install"

# -- custom basename --

WORK2="$(mktemp -d)"
trap 'rm -rf "$WORK2"' EXIT

snap download --target-directory="$WORK2" --basename=hw hello-world

test -f "$WORK2/hw.snap"
test -f "$WORK2/hw.assert"

# -- channel flag accepted (no error) --

WORK3="$(mktemp -d)"
trap 'rm -rf "$WORK3"' EXIT

snap download --target-directory="$WORK3" --channel=stable hello-world
ls "$WORK3"/hello-world_*.snap
