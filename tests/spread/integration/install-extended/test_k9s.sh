#!/usr/bin/env bash
# k8s tui, classic.

set -eux

snap install k9s

test -e /snap/k9s/current/meta/snap.yaml
test -L /snap/bin/k9s

snap run k9s version | grep -qi 'version'

# behavioural: `info` prints resolved config/log/data paths. exercises
# real init w/out entering the tui.
out=$(snap run k9s info)
echo "$out" | grep -qi 'configuration:'
echo "$out" | grep -qi 'logs:'
