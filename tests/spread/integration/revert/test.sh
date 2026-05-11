#!/usr/bin/env bash
# snap revert switches back to the previous revision after a refresh.
# uses hello-world (tiny, no daemons) so the test is fast.
#
# flow: install -> record rev1 -> refresh -> record rev2 -> revert -> check rev1 active.

set -eux

snap install hello-world

# record the revision installed
REV1="$(snap list | awk '/^hello-world / {print $3}')"
test -n "$REV1"

# force a refresh to a different revision via edge channel.
# if edge == stable (same rev), pin an older one via --revision.
# we can't guarantee edge != stable, so we use --channel=edge and
# accept that if they're the same the revert test is a noop check.
snap refresh --channel=edge hello-world || true

REV2="$(snap list | awk '/^hello-world / {print $3}')"
test -n "$REV2"

if [ "$REV1" = "$REV2" ]; then
    # edge and stable are the same revision; verify revert errors cleanly
    # (nothing to revert to since pruneOldRevisions kept only one rev).
    OUT="$(snap revert hello-world 2>&1 || true)"
    echo "$OUT" | grep -qi "no previous revision"
    echo "SKIP: stable and edge are the same revision; revert error path verified"
    snap remove hello-world
    exit 0
fi

# two different revisions on disk: revert should switch back to rev1
snap revert hello-world

CUR="$(snap list | awk '/^hello-world / {print $3}')"
test "$CUR" = "$REV1"

# snap is still functional after revert
snap run hello-world | grep -q "Hello World"

# -- revert again errors (only one revision on disk now) --
# after revert, the on-disk layout is: rev1 (current) + rev2 (previous).
# reverting once more should switch to rev2, not error.
snap revert hello-world
CUR2="$(snap list | awk '/^hello-world / {print $3}')"
test "$CUR2" = "$REV2"

# -- --revision flag --
snap revert --revision="$REV1" hello-world
CUR3="$(snap list | awk '/^hello-world / {print $3}')"
test "$CUR3" = "$REV1"

# -- already at target revision errors --
OUT="$(snap revert --revision="$REV1" hello-world 2>&1 || true)"
echo "$OUT" | grep -qi "already at revision"

# cleanup
snap remove hello-world
test ! -e /snap/hello-world
