#!/usr/bin/env bash
# core18 probe via node's 18/stable track. only test exercising core18 base.
# multi-app (node, npm, npx).

set -eux

snap install node --classic --channel=18/stable

# debug: dump state up front so truncated spread tail still has signal
echo "DEBUG: /snap/bin contents:"
ls -la /snap/bin/ || true
echo "DEBUG: /snap/node/current contents:"
ls -la /snap/node/current/ || true
echo "DEBUG: snap list:"
snap list || true

echo "STEP: meta/snap.yaml"
test -e /snap/node/current/meta/snap.yaml
echo "STEP: /snap/bin/node symlink"
test -L /snap/bin/node
echo "STEP: /snap/bin/node.npm symlink"
test -L /snap/bin/node.npm
echo "STEP: /snap/bin/node.npx symlink"
test -L /snap/bin/node.npx

echo "STEP: node --version"
snap run node --version | grep -qE '^v18\.'
echo "STEP: node.npm --version"
snap run node.npm --version | grep -qE '^[0-9]+\.'

echo "STEP: node eval 2+3"
[ "$(snap run node -e 'console.log(2+3)')" = "5" ]
echo "STEP: node eval json"
snap run node -e 'process.stdout.write(JSON.stringify({ok:true}))' | grep -q '"ok":true'
echo "STEP: node.npm config get registry"
snap run node.npm config get registry | grep -qE '^https?://'
echo "STEP: all passed"
