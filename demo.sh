#!/bin/sh

# refuse to run outside a container — this demo bypasses confinement entirely
if ! [ -f /.dockerenv ] && ! [ -f /run/.containerenv ] && ! grep -qE '(docker|lxc|kubepods|libpod|containerd)' /proc/1/cgroup 2>/dev/null; then
	echo "ERROR: demo.sh must run inside a container. it bypasses confinement and is unsafe on a real host." >&2
	exit 1
fi

set -ex
export PATH=/snap/bin:$PATH

# wait for snapd to be ready -- in the rock, pebble starts snapd as a
# service and `podman exec` may land before the socket appears.
i=0
while [ $i -lt 30 ] && [ ! -S /run/snapd.socket ]; do
    sleep 0.5
    i=$((i + 1))
done

snap install hello-world
hello-world
snap remove hello-world
