#!/usr/bin/env bash
# tiniest snap; exercises the "no base declared in snap.yaml" branch
# (install.go falls back to core22).

set -eux

snap install hello-world

test -e /snap/hello-world/current/meta/snap.yaml
test -L /snap/bin/hello-world

snap run hello-world | grep -q "Hello World"

# snap with no daemons: services should succeed and show empty table
snap services | grep -q "^Service"
! snap services | grep -q "^hello-world"
