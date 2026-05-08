#!/usr/bin/env bash
set -eux

snap install helm

test -e /snap/helm/current/meta/snap.yaml
test -L /snap/bin/helm

snap run helm version | grep -qi version
