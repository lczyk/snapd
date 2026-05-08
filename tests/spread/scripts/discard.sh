#!/usr/bin/env bash
# spread adhoc discard hook. spread only hands SPREAD_SYSTEM_ADDRESS
# back to discard, not the container name -- so we have to look up
# the container by IP. without this, parallel workers can't be torn
# down (every discard would target the same name). hack inherited
# verbatim from spread-bread.

set -e

echo "Discarding container at $SPREAD_SYSTEM_ADDRESS"

container_name=""
for cid in $(docker ps -a --filter "name=snap-spread-" --filter "network=bridge" --format '{{.ID}}'); do
    cname=$(docker inspect "$cid" --format '{{.Name}}' | sed 's/^\/\(.*\)/\1/')
    cip=$(docker inspect "$cid" --format '{{.NetworkSettings.Networks.bridge.IPAddress}}' || echo "")
    echo "Checking container: $cname with IP: $cip"
    if [ "$cip" = "$SPREAD_SYSTEM_ADDRESS" ]; then
        container_name="$cname"
        break
    fi
done

if [ -n "$container_name" ]; then
    echo "Removing container: $container_name"
    docker rm -f "$container_name" 2>/dev/null || true
else
    echo "No container found with IP address: $SPREAD_SYSTEM_ADDRESS"
    exit 1
fi
