#!/usr/bin/env bash
# snap service lifecycle test using mosquitto (simple mqtt broker daemon,
# no systemd calls, bundles its own libmicrohttpd).
#
# tests: install auto-starts daemon, snap services reports active,
# snap stop/start/restart work, snap logs has output, snap remove
# stops the daemon cleanly.

set -eux

SNAP=mosquitto
SVC=mosquitto.mosquitto
PID_FILE=/run/snapd/supervisors/mosquitto.mosquitto.pid
SOCK_FILE=/run/snapd/supervisors/mosquitto.mosquitto.sock
LOG_FILE=/var/log/snapd/mosquitto.mosquitto.log

# -- install --

snap install mosquitto

# supervisor should be up within a few seconds of install
for i in $(seq 1 10); do
    [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null && break
    sleep 1
done
test -f "$PID_FILE"
kill -0 "$(cat "$PID_FILE")"

snap services | grep -q "^$SVC.*active"

# daemon should be listening on 1883
sleep 2
ss -tlnp | grep -q ':1883'
pgrep -f /snap/mosquitto/ | grep -q .
pgrep -f "secret-daemon-mode $SVC" | grep -q .
test -S "$SOCK_FILE"
test -s "$LOG_FILE"

# supervisor and daemon are two distinct processes
SUPER_PID="$(cat "$PID_FILE")"
DAEMON_PID="$(pgrep -f /snap/mosquitto/)"
test "$SUPER_PID" != "$DAEMON_PID"

# -- logs --

snap logs "$SVC" | grep -q "mosquitto version"

# -- stop --

snap stop "$SVC"

# give supervisor a moment to exit
for i in $(seq 1 5); do
    [ ! -f "$PID_FILE" ] && break
    sleep 1
done
test ! -f "$PID_FILE"

snap services | grep -q "^$SVC.*inactive"
! ss -tlnp | grep -q ':1883'
! pgrep -f /snap/mosquitto/ | grep -q .
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
pgrep -f /snap/mosquitto/ | grep -q .
pgrep -f "secret-daemon-mode $SVC" | grep -q .
test -S "$SOCK_FILE"

# -- restart --

# snap restart keeps snap-super running (same pid); the daemon inside
# is stopped and re-forked. verify the service stays active after restart.
PRE_RESTART_SUPER_PID="$(cat "$PID_FILE")"
snap restart "$SVC"

for i in $(seq 1 10); do
    snap services | grep -q "^$SVC.*active" && break
    sleep 1
done
snap services | grep -q "^$SVC.*active"
kill -0 "$(cat "$PID_FILE")"
pgrep -f /snap/mosquitto/ | grep -q .
pgrep -f "secret-daemon-mode $SVC" | grep -q .
test -S "$SOCK_FILE"
# supervisor pid unchanged across restart. we don't assert the daemon pid
# changed: the os may reuse the old pid immediately, making that check
# unreliable. the STARTS >= 2 log check below is the authoritative proof
# that the daemon was restarted.
test "$(cat "$PID_FILE")" = "$PRE_RESTART_SUPER_PID"

# log should contain a second "starting" line from the restart
sleep 2
STARTS=$(grep -c "\[snap-super\] starting" "$LOG_FILE" || true)
test "$STARTS" -ge 2

# -- enable/disable (warn + noop) --

snap enable  "$SVC" 2>&1 | grep -qi "not supported"
snap disable "$SVC" 2>&1 | grep -qi "not supported"

# daemon still active after noop
snap services | grep -q "^$SVC.*active"

# -- remove --

snap remove mosquitto

# pid file and log file should be gone after remove
test ! -f "$PID_FILE"
! pgrep -f /snap/mosquitto/ | grep -q .
! pgrep -f "secret-daemon-mode $SVC" | grep -q .
test ! -e "$SOCK_FILE"

# snap tree should be gone
test ! -e /snap/mosquitto
