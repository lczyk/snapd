#!/usr/bin/env bash
# gnu hello -- different from hello-world, exercises core22-based snap
# w/ a single statically-named binary.

set -eux

snap install hello

test -e /snap/hello/current/meta/snap.yaml
test -L /snap/bin/hello

snap run hello | grep -qi "hello"
