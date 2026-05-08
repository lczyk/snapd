#!/usr/bin/env bash
# dynamic-link-heavy (ncurses, libtinfo, ...). exercises
# LD_LIBRARY_PATH wiring + base-fs symlinks under /lib/<triplet>.

set -eux

snap install htop

test -e /snap/htop/current/meta/snap.yaml
test -L /snap/bin/htop

snap run htop --version | grep -qi htop
