#!/usr/bin/env bash
set -eux

snap install chisel

test -e /snap/chisel/current/meta/snap.yaml
test -L /snap/bin/chisel

snap run chisel version | grep -qE '^v[0-9]+\.[0-9]+'
