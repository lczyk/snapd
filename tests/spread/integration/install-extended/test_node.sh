#!/usr/bin/env bash
# core18 probe via node's 18/stable track. only test exercising core18 base.
# multi-app (node, npm, npx).

set -eux

snap install node --classic --channel=18/stable

test -e /snap/node/current/meta/snap.yaml
test -L /snap/bin/node
test -L /snap/bin/npm
test -L /snap/bin/npx

snap run node --version | grep -qE '^v18\.'
snap run npm --version | grep -qE '^[0-9]+\.'

# behavioural: run real js. exercises v8 + libc from core18.
[ "$(snap run node -e 'console.log(2+3)')" = "5" ]
snap run node -e 'process.stdout.write(JSON.stringify({ok:true}))' | grep -q '"ok":true'
# npm: a local command that doesn't touch the network.
snap run npm config get registry | grep -qE '^https?://'
