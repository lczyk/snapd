#!/bin/sh
set -e
mkdir -p /run/snapd /snap /var/lib/snapd /var/lib/snapd/snaps /var/lib/snapd/cookie /snap/bin
/usr/local/bin/snapd &
SNAPD=$!
i=0
while [ $i -lt 30 ]; do
    [ -S /run/snapd.socket ] && break
    sleep 0.5
    i=$((i + 1))
done
[ -S /run/snapd.socket ] || { echo "snapd failed to come up"; exit 1; }

# don't exec the user command -- if we did, snapd (the bg child) would
# be reparented to pid 1, and pid 1 (pebble in the rock, tini in docker)
# would wait for it after the user command exits, hanging the container.
# instead, run as a foreground child and kill snapd ourselves on exit.
case "$1" in
    demo) /usr/local/bin/demo.sh; rc=$? ;;
    *)    "$@"; rc=$? ;;
esac
kill "$SNAPD" 2>/dev/null || true
wait "$SNAPD" 2>/dev/null || true
exit $rc
