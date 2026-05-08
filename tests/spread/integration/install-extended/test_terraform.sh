#!/usr/bin/env bash
set -eux

snap install terraform

test -e /snap/terraform/current/meta/snap.yaml
test -L /snap/bin/terraform

snap run terraform version | grep -qi terraform
