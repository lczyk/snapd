#!/usr/bin/env bash
# small classic snap with a real i/o roundtrip.

set -eux

snap install jq

test -e /snap/jq/current/meta/snap.yaml
test -L /snap/bin/jq

echo '{"a":1}' | snap run jq -r .a | grep -q '^1$'
