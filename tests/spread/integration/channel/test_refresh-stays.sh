#!/usr/bin/env bash
# refresh w/out --channel keeps the recorded channel.

set -eux

snap install --channel=edge hello-world
grep -q "^edge$" /snap/hello-world/.channel

snap refresh hello-world
grep -q "^edge$" /snap/hello-world/.channel
