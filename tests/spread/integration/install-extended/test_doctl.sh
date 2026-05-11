#!/usr/bin/env bash
# digitalocean cli, classic. no auth available -- exercise local-only paths.

set -eux

snap install doctl

test -e /snap/doctl/current/meta/snap.yaml
test -L /snap/bin/doctl

snap run doctl version | grep -qi 'doctl version'

# behavioural: subcommand tree (cobra) wires up.
snap run doctl compute --help | grep -qi 'droplet'
snap run doctl kubernetes --help | grep -qi 'cluster'
# negative: unknown subcommand exits non-zero.
! snap run doctl notarealsubcommand 2>/dev/null
