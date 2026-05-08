#!/usr/bin/env bash
# big strict snap (~113 MB). slow but it's a popular static site
# generator worth exercising in extended.

set -eux

snap install hugo

test -e /snap/hugo/current/meta/snap.yaml
test -L /snap/bin/hugo

snap run hugo version | grep -qi hugo
