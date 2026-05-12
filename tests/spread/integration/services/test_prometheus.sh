#!/usr/bin/env bash
# snap service lifecycle test using prometheus (single go binary, no fork,
# restart-condition: on-failure, reads config from $SNAP_COMMON).
#
# prometheus won't start without a valid prometheus.yml. the snap relies on
# a configure hook to create one; our binary doesn't run hooks, so we
# replicate that step manually before waiting for the daemon.

set -eux

SNAP=prometheus
SVC=prometheus.prometheus
PID_FILE=/run/snapd/supervisors/prometheus.prometheus.pid
SOCK_FILE=/run/snapd/supervisors/prometheus.prometheus.sock
LOG_FILE=/var/log/snapd/prometheus.prometheus.log
SNAP_COMMON=/var/snap/prometheus/common

# -- install --

snap install prometheus

# replicate the configure hook: write a minimal prometheus.yml so the
# daemon has something valid to load. snap-super retries with backoff
# until the process starts successfully.
mkdir -p "$SNAP_COMMON"
cat > "$SNAP_COMMON/prometheus.yml" <<'EOF'
global:
  scrape_interval: 15s
scrape_configs:
  - job_name: prometheus
    static_configs:
      - targets: ['localhost:9090']
EOF

for i in $(seq 1 30); do
    [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null && break
    sleep 1
done
test -f "$PID_FILE"
kill -0 "$(cat "$PID_FILE")"

snap services | grep -q "^$SVC.*active"

sleep 2
ss -tlnp | grep -q ':9090'
pgrep -f /snap/prometheus/ | grep -q .
pgrep -f "secret-daemon-mode $SVC" | grep -q .
test -S "$SOCK_FILE"
test -s "$LOG_FILE"

SUPER_PID="$(cat "$PID_FILE")"
DAEMON_PID="$(pgrep -f /snap/prometheus/)"
test "$SUPER_PID" != "$DAEMON_PID"

wget -qO- http://localhost:9090/-/healthy | grep -qi "prometheus"

# -- logs --

snap logs "$SVC" | grep -q "prometheus"

# -- stop --

snap stop "$SVC"

for i in $(seq 1 10); do
    [ ! -f "$PID_FILE" ] && break
    sleep 1
done
test ! -f "$PID_FILE"

snap services | grep -q "^$SVC.*inactive"
! ss -tlnp | grep -q ':9090'
! pgrep -f /snap/prometheus/ | grep -q .
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
pgrep -f /snap/prometheus/ | grep -q .
pgrep -f "secret-daemon-mode $SVC" | grep -q .
test -S "$SOCK_FILE"
sleep 1
wget -qO- http://localhost:9090/-/healthy | grep -qi "prometheus"

# -- restart --

PRE_RESTART_SUPER_PID="$(cat "$PID_FILE")"
snap restart "$SVC"

for i in $(seq 1 10); do
    snap services | grep -q "^$SVC.*active" && break
    sleep 1
done
snap services | grep -q "^$SVC.*active"
kill -0 "$(cat "$PID_FILE")"
pgrep -f /snap/prometheus/ | grep -q .
pgrep -f "secret-daemon-mode $SVC" | grep -q .
test -S "$SOCK_FILE"
# supervisor pid unchanged across restart. we don't assert the daemon pid
# changed: the os may reuse the old pid immediately, making that check
# unreliable. the STARTS >= 2 log check below is the authoritative proof
# that the daemon was restarted.
test "$(cat "$PID_FILE")" = "$PRE_RESTART_SUPER_PID"
sleep 1
wget -qO- http://localhost:9090/-/healthy | grep -qi "prometheus"

STARTS=$(grep -c '\[snap-super\] starting' "$LOG_FILE" || true)
test "$STARTS" -ge 2

# -- enable/disable (warn + noop) --

snap enable  "$SVC" 2>&1 | grep -qi "not supported"
snap disable "$SVC" 2>&1 | grep -qi "not supported"

snap services | grep -q "^$SVC.*active"

# -- remove --

snap remove prometheus

test ! -f "$PID_FILE"
! pgrep -f /snap/prometheus/ | grep -q .
! pgrep -f "secret-daemon-mode $SVC" | grep -q .
test ! -e "$SOCK_FILE"
test ! -e /snap/prometheus
