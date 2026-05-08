#!/usr/bin/env bash
set -eux

snap install shellcheck

test -e /snap/shellcheck/current/meta/snap.yaml
test -L /snap/bin/shellcheck

snap run shellcheck --version | grep -qi shellcheck
