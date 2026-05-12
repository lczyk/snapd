#!/usr/bin/env bash
# k9s: confirms install handles "metadata-only" snaps correctly.
#
# the k9s snap on the store (rev 155, latest/stable as of 2026-05) is
# upstream-broken: its meta/snap.yaml declares no `apps:` section at
# all, only metadata (name, version, base, grade, ...). the binary at
# bin/k9s exists inside the squashfs but no app entry points at it.
#
# real snapd 2.75.2 exhibits the same behaviour -- after `snap install
# k9s` there is no /snap/bin/k9s symlink, and `snap run k9s` errors
# with "snap \"k9s\" has no apps". the snap is unrunnable from PATH on
# any snapd. confirmed by installing on host: yaml is 407 bytes and
# ends at `grade: stable`. nothing about this is specific to the
# stripped-down build under test.
#
# we keep k9s in the variant set anyway, b/c it exercises the install
# path's handling of an apps-less snap: extract must succeed, wireBins
# must noop (no apps -> no symlinks, not an error), and `snap run`
# must surface the missing-apps condition cleanly.

set -eux

snap install k9s

# extract landed: meta + bin dirs present, but no /snap/bin wrapper
# (matches real snapd; see note above).
test -e /snap/k9s/current/meta/snap.yaml
test -e /snap/k9s/current/bin/k9s
[ ! -e /snap/bin/k9s ]

# yaml is metadata-only (no apps section). assert by grepping out the
# absence of the `apps:` top-level key, so this test starts failing
# loudly if upstream ever ships a fixed k9s snap -- at which point
# this script can be flipped to assert positive behaviour again.
! grep -qE '^apps:' /snap/k9s/current/meta/snap.yaml

# `snap run k9s` must error with the documented "has no apps" message
# rather than panicking, segfaulting, or silently no-op'ing. this is
# the load-bearing behaviour worth pinning. capture into a tmpfile
# rather than piping to tee -- a pipeline would mask snap's exit code
# behind tee's exit code.
set +e
snap run k9s help >/tmp/k9s.out 2>&1
rc=$?
set -e
cat /tmp/k9s.out
[ "$rc" -ne 0 ]
grep -q 'has no apps' /tmp/k9s.out
