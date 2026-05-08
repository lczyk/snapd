#!/usr/bin/env bash
# uses lzo compression, which no other lean-suite snap does.
# exercises rasky/go-lzo decompression in the squashfs reader.

set -eux

snap install btop

test -e /snap/btop/current/meta/snap.yaml
test -L /snap/bin/btop

snap run btop --version | grep -qi btop
