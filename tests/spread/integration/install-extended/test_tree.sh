#!/usr/bin/env bash
set -eux

snap install tree

test -e /snap/tree/current/meta/snap.yaml
test -L /snap/bin/tree

mkdir -p /tmp/spread-tree-test/a/b
touch /tmp/spread-tree-test/a/b/c.txt
snap run tree /tmp/spread-tree-test | grep -q "c.txt"
rm -rf /tmp/spread-tree-test
