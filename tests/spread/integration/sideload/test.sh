#!/usr/bin/env bash
# fetch a .snap blob via a normal store install, copy it out (snap
# remove deletes the cached blob), then sideload-install it. exercises
# the installLocal path: no store, no assertion verification, synthetic
# x1 revision.

set -eux

snap install hello-world
cp /var/lib/snapd/snaps/hello-world_*.snap /tmp/sideload-blob.snap
snap remove hello-world

# pre-condition: hello-world is gone, blob saved at /tmp.
test ! -e /snap/hello-world

snap install /tmp/sideload-blob.snap

test -e /snap/hello-world/current/meta/snap.yaml
test -L /snap/bin/hello-world

# sideloaded revisions are tagged xN; current should resolve to x1.
test "$(readlink /snap/hello-world/current)" = "x1"

snap run hello-world | grep -q "Hello World"

rm -f /tmp/sideload-blob.snap
