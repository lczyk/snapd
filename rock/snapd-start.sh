#!/usr/bin/bash
# pebble service wrapper -- create the runtime dirs that snapd expects
# and then exec it. /run is tmpfs so these can't be baked in at build.
set -e
mkdir -p /run/snapd /snap /var/lib/snapd/snaps /var/lib/snapd/cookie /snap/bin
exec /usr/local/bin/snapd
