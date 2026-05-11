#!/usr/bin/env bash
# k8s tui, classic.

set -eux

snap install k9s

test -e /snap/k9s/current/meta/snap.yaml
test -L /snap/bin/k9s

# behavioural: `info` prints resolved config/log/data paths + version.
# exercises real init w/out entering the tui. `version` subcommand is
# avoided -- it launches the interactive tui in some snap revisions and
# hangs headless.
out=$(snap run k9s info)
echo "$out" | grep -qi 'configuration:'
echo "$out" | grep -qi 'logs:'
echo "$out" | grep -qi 'Version:'
