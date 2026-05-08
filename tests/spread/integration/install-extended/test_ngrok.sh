#!/usr/bin/env bash
# ngrok start needs an auth token; just run version.

set -eux

snap install ngrok

test -e /snap/ngrok/current/meta/snap.yaml
test -L /snap/bin/ngrok

snap run ngrok version | grep -qi ngrok
