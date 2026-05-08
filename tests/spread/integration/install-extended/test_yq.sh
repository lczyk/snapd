#!/usr/bin/env bash
set -eux

snap install yq

test -e /snap/yq/current/meta/snap.yaml
test -L /snap/bin/yq

echo 'a: 1' | snap run yq '.a' | grep -q '^1$'
