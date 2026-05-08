#!/usr/bin/env bash
# multi-app snap -- exercises wireBins on more than just the bare
# /snap/bin/<name> shim.

set -eux

snap install nvim

test -e /snap/nvim/current/meta/snap.yaml
test -L /snap/bin/nvim

snap run nvim --version | grep -qi nvim
