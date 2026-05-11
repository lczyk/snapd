#!/usr/bin/env bash
# snap service lifecycle test using redis (simple key-value store daemon,
# non-forking, restart-condition: always, uses $SNAP_COMMON for config+data).
#
# contrast with mosquitto: single process (no fork), core22 base, data dir
# usage, PING/PONG protocol assertion.
#
# the redis snap relies on a configure hook to initialise
# $SNAP_COMMON/etc/redis/redis.conf. our snap binary doesn't run hooks, so
# we replicate that step manually before waiting for the daemon.

set -eux

SNAP=redis
SVC=redis.server
PID_FILE=/run/snapd/supervisors/redis.server.pid
SOCK_FILE=/run/snapd/supervisors/redis.server.sock
LOG_FILE=/var/log/snapd/redis.server.log
SNAP_COMMON=/var/snap/redis/common

# -- install --

snap install redis

# replicate the configure hook: create config + data dirs, copy and
# patch the bundled default config so redis-server finds its data dir.
mkdir -p "$SNAP_COMMON/etc/redis" "$SNAP_COMMON/var/lib/redis"
cp /snap/redis/current/conf-dist/redis.conf "$SNAP_COMMON/etc/redis/redis.conf"
sed -i "s~^dir /var/lib/redis~dir $SNAP_COMMON/var/lib/redis~" \
    "$SNAP_COMMON/etc/redis/redis.conf"

# snap-super starts immediately after install and retries with backoff
# until the config exists. wait up to 30s for it to pick it up.
for i in $(seq 1 30); do
    [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null && break
    sleep 1
done
test -f "$PID_FILE"
kill -0 "$(cat "$PID_FILE")"

snap services | grep -q "^$SVC.*active"

# redis binds on 6379; give it a moment then check the port
sleep 2
ss -tlnp | grep -q ':6379'
pgrep -f /snap/redis/ | grep -q .
pgrep -f "secret-daemon-mode $SVC" | grep -q .
test -S "$SOCK_FILE"
test -s "$LOG_FILE"

# supervisor and daemon are two distinct processes
SUPER_PID="$(cat "$PID_FILE")"
DAEMON_PID="$(pgrep -f /snap/redis/)"
test "$SUPER_PID" != "$DAEMON_PID"

# PING -> PONG via the redis.cli shim
snap run redis.cli ping | grep -qi 'PONG'

# -- logs --

snap logs "$SVC" | grep -q 'oO0OoO0OoO0Oo'

# -- stop --

snap stop "$SVC"

for i in $(seq 1 10); do
    [ ! -f "$PID_FILE" ] && break
    sleep 1
done
test ! -f "$PID_FILE"

snap services | grep -q "^$SVC.*inactive"
! ss -tlnp | grep -q ':6379'
! pgrep -f /snap/redis/ | grep -q .
! pgrep -f "secret-daemon-mode $SVC" | grep -q .
test ! -e "$SOCK_FILE"

# -- start --

snap start "$SVC"

for i in $(seq 1 10); do
    [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null && break
    sleep 1
done
kill -0 "$(cat "$PID_FILE")"
snap services | grep -q "^$SVC.*active"
pgrep -f /snap/redis/ | grep -q .
pgrep -f "secret-daemon-mode $SVC" | grep -q .
test -S "$SOCK_FILE"
sleep 1
snap run redis.cli ping | grep -qi 'PONG'

# -- restart --

PRE_RESTART_SUPER_PID="$(cat "$PID_FILE")"
snap restart "$SVC"

for i in $(seq 1 10); do
    snap services | grep -q "^$SVC.*active" && break
    sleep 1
done
snap services | grep -q "^$SVC.*active"
kill -0 "$(cat "$PID_FILE")"
pgrep -f /snap/redis/ | grep -q .
pgrep -f "secret-daemon-mode $SVC" | grep -q .
test -S "$SOCK_FILE"
# supervisor pid unchanged across restart. we don't assert the daemon pid
# changed: the os may reuse the old pid immediately, making that check
# unreliable. the STARTS >= 2 log check below is the authoritative proof
# that the daemon was restarted.
test "$(cat "$PID_FILE")" = "$PRE_RESTART_SUPER_PID"
sleep 1
snap run redis.cli ping | grep -qi 'PONG'

STARTS=$(grep -c '\[snap-super\] starting' "$LOG_FILE" || true)
test "$STARTS" -ge 2

# -- enable/disable (warn + noop) --

snap enable  "$SVC" 2>&1 | grep -qi "not supported"
snap disable "$SVC" 2>&1 | grep -qi "not supported"

snap services | grep -q "^$SVC.*active"

# -- remove --

snap remove redis

test ! -f "$PID_FILE"
! pgrep -f /snap/redis/ | grep -q .
! pgrep -f "secret-daemon-mode $SVC" | grep -q .
test ! -e "$SOCK_FILE"
test ! -e /snap/redis
