#!/usr/bin/env bash
# tiniest core24 snap -- proves core24 base works.

set -eux

snap install rustup

test -e /snap/rustup/current/meta/snap.yaml
test -L /snap/bin/rustup

snap run rustup --version | grep -qi rustup
