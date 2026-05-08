# snap (single-binary prototype)

a chainsawed fork of [snapcore/snapd](https://github.com/snapcore/snapd) on the
`one-snap-to-rule-them-all` branch. one static go binary -- ~12 MB, no daemon,
no state file, no cgo, no confinement -- that can install (download +
verify + unsquash) and run snaps inside a container.

upstream snapd is a long-running root daemon coordinating apparmor / seccomp /
mount-namespace / cgroup confinement, a state engine, an api socket, and a
fleet of cli tools. this fork keeps the parts that talk to the snap store and
parse `.snap` files, and throws the rest away. ~1.2k LoC under `cmd/snap/`,
plus a few patched packages it pulls in.

## design

- **single binary, no daemon** `/usr/bin/snap` is the only thing that runs.
  no `snapd`, no `snapctl`, no `snap-confine`, no dbus, no systemd units.
- **container is the security boundary** every snap is treated as classic.
  no apparmor, no seccomp, no mount ns. you isolate by running this thing
  inside docker / podman / a rock, not by trusting the snap.
- **state on disk, no state engine** `/snap/<name>/<rev>/` for extracted
  snaps, `/snap/<name>/current` symlink for the active rev, `/var/snap/<name>/`
  for per-snap data, `/var/lib/snapd/assertions/` for the persistent assertion
  db, `/var/lib/snapd/snaps/` for cached downloads. that's the whole thing.
- **assertions still verified** `snap-revision -> account-key -> account` and
  `snap-declaration -> account-key -> account` chains are walked against
  canonical's brand keys before extraction. sha3-384 of the blob is
  cross-checked against the revision assertion.
- **base snap = userland** the bare container ships with `/usr/bin/snap` and
  ca-certs and nothing else -- no libc, no `/bin/sh`, no `ld-linux-*`. the
  first install pulls a base snap (defaults to `core22`) and symlinks
  `/lib/ld-linux-*`, `/lib/<triplet>`, `/bin/sh`, and walks
  `/usr/bin /bin /usr/sbin /sbin` populating host paths. that's how every
  later snap gets a working dynamic loader and shell.
- **`/snap/bin/<x>` shims** on install, `apps:` entries get symlinked into
  `/snap/bin/<x>` pointing at the snap binary itself. when invoked via the
  shim, `os.Args[0]` triggers shim mode -- equivalent to `snap run <x>`.
- **cli-only, no gui** the target is headless container interaction:
  `docker exec` / `podman exec` / `kubectl exec` into a container, run a snap.
  there's no x11 / wayland socket, no dbus, no pulseaudio, no desktop
  portals, no gtk / qt theming, no fontconfig, no fonts, no `/dev/dri`.
  base snaps don't ship those libs either -- they're the runtime userland,
  not a desktop. so gui snaps (chromium, firefox, vlc, gimp, ...) are out
  of scope by construction: even if they extracted cleanly, they'd have
  nothing to render to and nothing to talk to. cli snaps only.
- **squashfs reader, pure go** patched in-tree to fix several bugs surfaced by
  real-world snaps (file truncation past block 56, fragment table absolute
  offsets, metadata-block boundary, lzo support via `rasky/go-lzo`).
  compression: gzip, xz, zstd, lzo. lzma + lz4 not yet wired.

## cli

```
snap install [--channel=<chan>] <name>   # store install (default: latest/stable)
snap install ./<file>.snap               # sideload, unverified
snap run <name>                          # run the default app
snap refresh [--channel=<chan>] [<name>...]
                                         # re-pull + re-extract; w/out --channel,
                                         # stays on the recorded channel
snap remove <name>                       # rm -rf /snap/<name> + /var/snap/<name> + shims
snap list                                # what's installed (incl. tracking channel)
snap info <name>                         # local fallback to remote
snap find <query>                        # search the store
snap help                                # this
```

`/snap/bin/<x>` symlinks make `<x> ...` equivalent to `snap run <x> ...` --
no need to type `snap run` in the shell.

## working snaps

verified to install and run on arm64 inside both docker (scratch) and a
rock (bare base):

| snap | what | base |
| --- | --- | --- |
| `hello-world` | smoke test | core (auto) |
| `hello` | gnu hello | core22 |
| `jq` | json parser | core22 |
| `yq` | yaml parser | core22 |
| `tree` | tree(1) | core22 |
| `htop` | process viewer | core22 |
| `btop` | nicer top | core22 |
| `ngrok` | tunnel | core22 |
| `terraform` | iac | core22 |
| `kubectl` | k8s cli | core22 |
| `helm` | k8s pkg manager | core22 |
| `snapd` | itself, kinda | core22 |
| `core` / `core20` / `core22` / `core24` | base snaps | -- |

gui snaps (chromium, firefox, vlc, ...) are explicitly out of scope -- see
the cli-only design note above. chromium happens to also fail to extract on
a specific large xz block; the improved squashfs error messages now point
at the on-disk bytes to inspect, but it wouldn't run anyway.

## usage

build:

```
make build       # ./bin/snap, static, ~12 MB
```

docker (scratch base, fastest iteration loop):

```
make docker                              # build the image
make docker-install SNAP=jq              # install one snap
make docker-shell BASE=core22            # bootstrap a base + drop into /bin/sh
make docker-wipe                         # nuke state volumes
```

rock (bare base + chisel ca-certs slice, via rockcraft + podman):

```
make rock                                # build the .rock OCI archive
make rock-install SNAP=jq                # install one snap
make rock-shell BASE=core22              # bootstrap + interactive shell
make rock-wipe                           # nuke state volumes
```

state persists across `*-install` / `*-shell` invocations via named volumes
(`snap-poc-state-*`, `snap-rock-state-*`); `*-wipe` removes them.

## what's not here

no confinement. no interfaces. no plugs / slots. no hooks. no services /
daemons (might come later as disowned supervisor children, but not implemented
upstream-style). no user data migration. no notices / change log. no api
socket. no `snap-confine`. no apparmor / seccomp / cgroup integration. no
udev rules. no systemd units. no model assertion / device serial. no
auto-refresh (no daemon to schedule it -- `snap refresh` is manual). no
quotas. no themes / desktop integration. no recovery mode. no `core` boot
logic. no ubuntu core. no tests (yet). 90+% of the upstream surface area
is gone.

manual refresh + channels *are* supported -- `snap install --channel=edge foo`
records the channel under `/snap/foo/.channel`, and a later `snap refresh foo`
stays on edge unless `--channel=` is passed again.

## license

GPL-3.0, same as upstream snapd. see [COPYING](COPYING).
