#!/bin/sh
set -e
mkdir -p /run/snapd /snap /var/lib/snapd /var/lib/snapd/snaps /var/lib/snapd/cookie
/usr/local/bin/snapd &
SNAPD=$!
for i in $(seq 1 30); do
    [ -S /run/snapd.socket ] && break
    sleep 0.5
done
[ -S /run/snapd.socket ] || { echo "snapd failed to come up"; exit 1; }
case "$1" in
    demo) exec /usr/local/bin/demo.sh ;;
    *)    exec "$@" ;;
esac
