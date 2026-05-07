#!/bin/sh

# refuse to run outside a container — this demo bypasses confinement entirely
if ! [ -f /.dockerenv ] && ! grep -qE '(docker|lxc|kubepods|libpod|containerd)' /proc/1/cgroup 2>/dev/null; then
	echo "ERROR: demo.sh must run inside a container. it bypasses confinement and is unsafe on a real host." >&2
	exit 1
fi

set -ex
export PATH=/snap/bin:$PATH
snap install hello-world
hello-world
snap remove hello-world
