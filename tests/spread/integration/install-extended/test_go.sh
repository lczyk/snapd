#!/usr/bin/env bash
# classic core24, ~59 MB. covers the go toolchain.

set -eux

snap install go

test -e /snap/go/current/meta/snap.yaml
test -L /snap/bin/go

snap run go version | grep -qi 'go version'
