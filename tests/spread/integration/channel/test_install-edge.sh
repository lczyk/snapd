#!/usr/bin/env bash
# install --channel=edge records the channel under /snap/<name>/.channel.

set -eux

snap install --channel=edge hello-world

test -e /snap/hello-world/current/meta/snap.yaml
test -f /snap/hello-world/.channel
grep -q "^edge$" /snap/hello-world/.channel

snap run hello-world | grep -q "Hello World"
