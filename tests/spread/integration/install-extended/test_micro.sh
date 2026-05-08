#!/usr/bin/env bash
# only test that exercises a core20 base.

set -eux

snap install micro

test -e /snap/micro/current/meta/snap.yaml
test -L /snap/bin/micro

# micro --version prints "Version: <semver>" -- no snap-name string.
snap run micro --version | grep -qE '^Version: [0-9]+\.'
