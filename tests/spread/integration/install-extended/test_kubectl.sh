#!/usr/bin/env bash
# --client avoids needing a cluster.

set -eux

snap install kubectl

test -e /snap/kubectl/current/meta/snap.yaml
test -L /snap/bin/kubectl

snap run kubectl version --client | grep -qi "client version"
