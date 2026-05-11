#!/usr/bin/env bash
# python-bundled classic. exercises interpreter + sizable lib tree unpack.

set -eux

snap install yt-dlp

test -e /snap/yt-dlp/current/meta/snap.yaml
test -L /snap/bin/yt-dlp

snap run yt-dlp --version | grep -qE '^[0-9]{4}\.'

# behavioural: list-extractors reads the bundled extractor plugins.
# proves the python plugin discovery walks the snap's site-packages tree.
out=$(snap run yt-dlp --list-extractors)
echo "$out" | grep -qi '^youtube$'
# sanity: many extractors -- not just a stub.
[ "$(echo "$out" | wc -l)" -gt 100 ]
