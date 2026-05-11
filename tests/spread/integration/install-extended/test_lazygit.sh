#!/usr/bin/env bash
# classic tui, go binary. tui can't run headless, so behavioural coverage
# is via the non-interactive subcommands.

set -eux

snap install lazygit

test -e /snap/lazygit/current/meta/snap.yaml
test -L /snap/bin/lazygit

snap run lazygit --version | grep -qi 'commit='

# behavioural: print resolved config dir/path. exercises real startup
# path (config + log dirs) without entering the tui.
snap run lazygit --print-config-dir | grep -qE '/lazygit$'
snap run lazygit --config | grep -qi 'gui:'
