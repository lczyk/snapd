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
case "$1" in
    demo) exec /usr/local/bin/demo.sh ;;
    *)    exec "$@" ;;
esac
