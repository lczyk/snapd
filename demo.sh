#!/bin/sh
set -ex
echo "=== snapd no-systemd prototype ==="
snap version
echo "=== seeding status ==="
snap changes 2>/dev/null || true
echo "=== installed snaps ==="
snap list 2>/dev/null || true
echo "=== trying store install of hello-world ==="
snap install hello-world
echo "=== running hello-world ==="
hello-world
echo "=== removing hello-world ==="
snap remove hello-world
echo "=== done ==="
