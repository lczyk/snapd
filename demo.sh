#!/bin/sh
set -ex
export PATH=/snap/bin:$PATH
snap install hello-world
hello-world
snap remove hello-world
