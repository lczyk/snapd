#!/usr/bin/env bash
# install -> list shows it -> remove -> list does not.

set -eux

snap install hello-world

snap list | grep -q "^hello-world "

snap remove hello-world

# /snap/hello-world should be gone. list either errors or does not
# include the snap.
test ! -e /snap/hello-world
! snap list | grep -q "^hello-world "
