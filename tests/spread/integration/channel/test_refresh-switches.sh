#!/usr/bin/env bash
# refresh --channel=<other> switches the recorded channel.

set -eux

snap install --channel=edge hello-world
grep -q "^edge$" /snap/hello-world/.channel

snap refresh --channel=stable hello-world
grep -q "^stable$" /snap/hello-world/.channel
