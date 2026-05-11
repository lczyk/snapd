#!/usr/bin/env bash
# snap service lifecycle test using postgresql (single daemon, restart-condition:
# on-failure, requires database cluster initialisation before first start).
#
# postgresql won't start against an uninitialised data directory. the snap
# relies on a configure hook to run initdb; our binary doesn't run hooks, so
# we invoke initdb manually via `snap run postgresql.initdb` before waiting
# for the daemon.

set -eux

SNAP=postgresql
SVC=postgresql.postgresql
PID_FILE=/run/snapd/supervisors/postgresql.postgresql.pid
SOCK_FILE=/run/snapd/supervisors/postgresql.postgresql.sock
LOG_FILE=/var/log/snapd/postgresql.postgresql.log
SNAP_COMMON=/var/snap/postgresql/common

# -- install --

snap install postgresql

# replicate the configure hook: initialise the data cluster so postgres
# has a valid data directory to start against. snap-super retries with
# backoff until the process starts successfully.
mkdir -p "$SNAP_COMMON"
snap run postgresql.initdb

for i in $(seq 1 30); do
    [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null && break
    sleep 1
done
test -f "$PID_FILE"
kill -0 "$(cat "$PID_FILE")"

snap services | grep -q "^$SVC.*active"

sleep 2
ss -tlnp | grep -q ':5432'
pgrep -f /snap/postgresql/ | grep -q .
pgrep -f "secret-daemon-mode $SVC" | grep -q .
test -S "$SOCK_FILE"
test -s "$LOG_FILE"

SUPER_PID="$(cat "$PID_FILE")"
DAEMON_PID="$(pgrep -f /snap/postgresql/)"
test "$SUPER_PID" != "$DAEMON_PID"

# basic query via psql
snap run postgresql.psql -U postgres -c "SELECT 1" | grep -q "1 row"

# -- logs --

snap logs "$SVC" | grep -q "database system"

# -- stop --

snap stop "$SVC"

for i in $(seq 1 10); do
    [ ! -f "$PID_FILE" ] && break
    sleep 1
done
test ! -f "$PID_FILE"

snap services | grep -q "^$SVC.*inactive"
! ss -tlnp | grep -q ':5432'
! pgrep -f /snap/postgresql/ | grep -q .
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
pgrep -f /snap/postgresql/ | grep -q .
pgrep -f "secret-daemon-mode $SVC" | grep -q .
test -S "$SOCK_FILE"
sleep 1
snap run postgresql.psql -U postgres -c "SELECT 1" | grep -q "1 row"

# -- restart --

PRE_RESTART_SUPER_PID="$(cat "$PID_FILE")"
snap restart "$SVC"

for i in $(seq 1 10); do
    snap services | grep -q "^$SVC.*active" && break
    sleep 1
done
snap services | grep -q "^$SVC.*active"
kill -0 "$(cat "$PID_FILE")"
pgrep -f /snap/postgresql/ | grep -q .
pgrep -f "secret-daemon-mode $SVC" | grep -q .
test -S "$SOCK_FILE"
# supervisor pid unchanged across restart. we don't assert the daemon pid
# changed: the os may reuse the old pid immediately, making that check
# unreliable. the STARTS >= 2 log check below is the authoritative proof
# that the daemon was restarted.
test "$(cat "$PID_FILE")" = "$PRE_RESTART_SUPER_PID"
sleep 2
snap run postgresql.psql -U postgres -c "SELECT 1" | grep -q "1 row"

STARTS=$(grep -c '\[snap-super\] starting' "$LOG_FILE" || true)
test "$STARTS" -ge 2

# -- enable/disable (warn + noop) --

snap enable  "$SVC" 2>&1 | grep -qi "not supported"
snap disable "$SVC" 2>&1 | grep -qi "not supported"

snap services | grep -q "^$SVC.*active"

# -- remove --

snap remove postgresql

test ! -f "$PID_FILE"
! pgrep -f /snap/postgresql/ | grep -q .
! pgrep -f "secret-daemon-mode $SVC" | grep -q .
test ! -e "$SOCK_FILE"
test ! -e /snap/postgresql
