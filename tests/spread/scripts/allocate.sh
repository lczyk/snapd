#!/usr/bin/env bash
# spread adhoc allocate hook. spawns an sshd-bearing docker container,
# bind-mounts a host-side blob cache at /var/lib/snapd/snaps so
# downloads survive the per-task wipe (project-level restore-each),
# waits for sshd to come up, then prints the container's bridge IP
# back to spread via ADDRESS.

set -e

flavour=$(echo "$SPREAD_SYSTEM" | cut -d- -f1)  # noble-arm64 -> noble
arch=$(echo "$SPREAD_SYSTEM" | cut -d- -f2)     # noble-arm64 -> arm64
echo "flavour: $flavour"
echo "arch: $arch"

image="snap-spread-sshd-$flavour-$arch"
echo "image: $image"

# unique container name per worker. SPREAD_WORKER starts at 0 and is
# stable per backend/system, so there's no race vs. the flock-counter
# pattern. fall back to a random suffix if it's somehow unset.
worker="${SPREAD_WORKER:-$RANDOM}"
container_name="snap-spread-${SPREAD_SYSTEM}-${worker}"
echo "container_name: $container_name"

# host-side blob cache. survives container teardown so re-runs of the
# spread suite skip the download step. gitignored.
cache_dir="$(pwd)/tests/spread/.cache/snaps"
mkdir -p "$cache_dir"
echo "cache_dir: $cache_dir"

# nuke any stale container with the same name (previous run that didn't
# tear down cleanly).
docker rm -f "$container_name" >/dev/null 2>&1 || true

docker run \
    --rm \
    --platform "linux/$arch" \
    -e DEBIAN_FRONTEND=noninteractive \
    -e "usr=$SPREAD_SYSTEM_USERNAME" \
    -e "pass=$SPREAD_SYSTEM_PASSWORD" \
    -v "$cache_dir:/var/lib/snapd/snaps" \
    --name "$container_name" \
    -d "$image"

until docker exec "$container_name" pgrep sshd >/dev/null; do sleep 1; done

ADDRESS "$(docker inspect "$container_name" --format '{{.NetworkSettings.Networks.bridge.IPAddress}}')"
