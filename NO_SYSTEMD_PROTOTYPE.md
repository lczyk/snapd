# no-systemd prototype: snapd in a vanilla docker container

throwaway proof of principle. this branch demonstrates that snapd's coupling to systemd is incidental, not structural -- a single docker image where `snap install hello-world && hello-world && snap remove hello-world` works without systemd anywhere in the picture. the resulting code never ships and has no back-compat, upgrade, or test-suite obligation. the deliverable is the working demo plus a small enough codebase that a reviewer can read it and conclude "yeah, snapd doesn't need systemd."

## demo target

inside the resulting docker image, this exact sequence works:

```sh
docker run --rm --device /dev/fuse --cap-add SYS_ADMIN snapd-poc demo
# -> snap install hello-world; hello-world; snap remove hello-world; container exits

docker run --rm -it --device /dev/fuse --cap-add SYS_ADMIN snapd-poc bash
# -> drops into a shell, snapd already running, drive commands manually
```

`SYS_ADMIN` is needed for fuse + the squashfuse mount we use; nothing else needs elevated privilege after the deletions below. no `--privileged`, no `apparmor=unconfined`, no `seccomp=unconfined`.

## scope

**in scope**: install / run / remove of one strictly-confined-snap, with confinement bypassed (see q3 below). assertion verification + store interaction over the network. squashfuse-based mounts. seeding via `snap prepare-image`.

**out of scope**: snap services / timers / sockets / dbus activation. apparmor + seccomp enforcement. ubuntu core boot integration, fde, recovery, gadget, kernel snaps. user session agents. multi-user. refresh / hold / channel logic beyond what `snap install` happens to exercise. rocks / rockcraft packaging (cheap follow-up once the docker version works). unit tests + spread suites (scorched-earth -- deletions take their tests with them).

## design decisions (and why)

ten architectural forks were settled before this plan was written. the short version:

| q | decision | reasoning |
|---|---|---|
| 1. removal scope | full deletion, no build flag | throwaway prototype; back-compat is not a goal |
| 2. demo bar | install + run + remove of hello-world only | smallest scope that proves the thesis |
| 3. confinement | bypass snap-confine entirely; exec the snap binary directly | strict confinement in docker is a rabbit hole orthogonal to "snapd needs systemd?"; bypass keeps the demo focused |
| 4. delete vs neuter | chainsaw obvious dead subtrees + scalpel install path | compile-clean as the floor; tests are casualties |
| 5. squashfs mount | squashfuse (userspace) | avoids `--privileged`; needs only `SYS_ADMIN` + `/dev/fuse` |
| 6. pid 1 | `tini` in front of snapd | snapd doesn't grow pid-1 awareness; not relevant to the thesis |
| 7. demo flow | scripted demo `cmd` + interactive shell `cmd` | one image, both modes |
| 8. interface backends | stub `Setup() / Remove() / Connect() / Disconnect()` to no-op | minimum-effort path; no on-disk profiles, no enforcement |
| 9. seeding | `snap prepare-image --classic` at image build time | seeding is fiddly enough that hand-rolling it is more code than just running the official tool |
| 10. rocks | docker first; rocks is a thin sibling once docker works | snapd is portable across both supervisors without code change |

## architecture

the running picture inside the container, top to bottom:

- **pid 1 = `tini`** (vendored, ~20kb). handles signal forwarding + zombie reaping. snapd doesn't need to know it exists.
- **tini -> `/usr/local/bin/entrypoint.sh`**. starts `snapd &` in the background, polls for `/run/snapd.socket` (~2s), then either `exec bash` (interactive, default `CMD`) or `exec /usr/local/bin/demo.sh` (canned, when `CMD` is `demo`).
- **snapd**: a normal long-lived go process. opens its own `/run/snapd.socket` at startup -- no socket activation, no `sd_notify`. on first boot, walks the seed at `/var/lib/snapd/seed/` (preseeded at image build time via `snap prepare-image --classic`) and marks itself seeded with a `generic-classic` model + `core24`.
- **`snap install hello-world`**: hits the store over the network, pulls `hello-world.snap` to `/var/lib/snapd/snaps/`, mounts via squashfuse at `/snap/hello-world/<rev>/`, runs the install task chain (with all interface backends stubbed).
- **`hello-world`** (the binary): `/snap/bin/hello-world` -> `snap run hello-world`. `snap run` is patched to short-circuit past the dbus-to-systemd cgroup placement step and past `exec snap-confine` -- it directly resolves the `snap.yaml` command and `exec`s the snap's binary with the right env (`$SNAP`, `$SNAP_DATA`, `$SNAP_USER_DATA`).
- **`snap remove hello-world`**: reverse of install -- terminates the squashfuse process, deletes files, updates `state.json`.

what's gone vs current snapd: every systemd-shaped abstraction. no service supervisor inside snapd, no unit-file generation, no journal stream, no `.mount` / `.service` / `.socket` / `.timer` files anywhere.

## deletion list (chainsaw)

these subtrees and files are deleted wholesale. tests in the same package go with them. shell-pasteable form:

```sh
# packaging / units
rm -rf data/systemd
rm -rf data/systemd-user
rm -rf data/dbus
rm -rf data/polkit

# unit-file generators
rm wrappers/dbus.go
rm wrappers/internal/service_unit_gen.go
rm wrappers/internal/service_socket_gen.go
rm wrappers/internal/service_timer_gen.go
rm wrappers/internal/service_slice_gen.go
rm wrappers/internal/journal_conf_gen.go
# (also remove their *_test.go siblings)

# core systemd / journald glue
rm -rf systemd
rm -rf usersession

# boot / uc / fde / device binaries
rm -rf cmd/snap-bootstrap
rm -rf cmd/snap-failure
rm -rf cmd/snap-recovery-chooser
rm -rf cmd/snapd-apparmor
rm -rf cmd/snapd-generator
rm -rf cmd/snap-fde-keymgr
rm -rf cmd/snap-update-ns
rm -rf cmd/snap-confine
rm -rf cmd/snap-discard-ns
rm -rf cmd/snap-gpio-helper
rm -rf cmd/snap-seccomp

# overlord subtrees that depend on the above
rm -rf overlord/fdestate
# overlord/devicestate: gut, don't delete -- see surgery list

# interface backends with systemd / dbus footprint
rm -rf interfaces/systemd
rm -rf interfaces/dbus
rm interfaces/builtin/dbus.go interfaces/builtin/dbus_test.go

# cgroup tracking via dbus -> systemd
rm sandbox/cgroup/tracking.go sandbox/cgroup/tracking_test.go

# misc
rm overlord/configstate/configcore/journal.go overlord/configstate/configcore/journal_test.go
```

after this, `go build ./cmd/snapd ./cmd/snap` is broken and won't recover until phase 1 chases all the importers.

## surgery list (scalpel)

these files keep existing but get patched in place. this is where the demo is made to work.

### install path: mount handling

**`overlord/snapstate/backend/mountunit.go`** -- replace the body, keep the function signatures:
- `addMountUnit(...)`: spawn `squashfuse <snapfile> <mountpoint>` as a child process. record the pid in `/run/snapd/mounts/<unit-name>.pid`. no `.mount` file is written.
- `removeMountUnit(...)`: read pid from `/run/snapd/mounts/<unit-name>.pid`, send `SIGTERM`, `wait`, then `fusermount -u <mountpoint>`.
- `RemoveContainerMountUnits(...)`: same as above, applied to the matching origin filter.

note: the `daemonReloadLock` mutex (was at `systemd/systemd.go:71`) is gone with the package. confirm nothing in `overlord/` relied on its serialisation properties.

### run path: cmd/snap/cmd_run.go

short-circuit two things:
- the call to `cgroupCreateTransientScopeForTracking` (was at `cmd/snap/cmd_run.go:1925`) -- replace with a no-op. this is the dbus-to-systemd cgroup placement step.
- the `exec snap-confine` branch -- the existing code already builds an env for snap-confine; reuse that, but `unix.Exec` the resolved app command directly instead of snap-confine. the resolved command comes from `snap.AppInfo.Command` (already parsed from `snap.yaml`).

### daemon startup: cmd/snapd/main.go + daemon/daemon.go

- delete the systemd-fd-passing branch in the listener-acquisition code; always `net.Listen("unix", "/run/snapd.socket")` directly.
- delete the `sdNotify("READY=1")` and `sdNotify("STOPPING=1")` calls.
- delete the `Type=notify` watchdog plumbing.

### interfaces orchestration: overlord/ifacestate/

- in each remaining interface backend (`apparmor`, `seccomp`, `mount`, `kmod`, `polkit`, `udev`, `apparmorprompting`), neuter `Setup() / Remove() / Connect() / Disconnect() / NewSpecification()` to return immediately with no work. **don't** delete the backend packages -- `ifacestate` registers them by name and the orchestration relies on that. just make the methods do nothing.
- the `setup-profiles` task chain step keeps running but writes nothing to disk.
- `interfaces/systemd/` and `interfaces/dbus/` are deleted (chainsaw); remove their registration from `overlord/ifacestate/handlers.go` (or wherever).

### hooks: overlord/hookstate/

- hello-world has no hooks; orchestration starts hook tasks but hookstate skips when no script exists. nothing to do for the demo.
- nonetheless, in `hookmgr.go` the hook execution path that wraps in `systemd-run` / `snap-confine` should be neutered: if a hook script ever runs (unlikely for our demo), it does so as a direct fork from snapd. keep this minimal -- only patch what the install path actually hits.

### device / seeding: overlord/devicestate/

`overlord/devicestate` is gutted-not-deleted, because seeding lives there.
- keep: `firstboot*.go` entrypoints, `doMarkSeeded`, basic model registration.
- delete: remodel logic, boot-asset tracking, gadget-update, recovery, uc20-isms, fde reseal hooks (already gone with `fdestate`).

## container packaging

a single `Dockerfile` in the repo root. multi-stage build:

### stage 1: builder

ubuntu:24.04. `apt install golang-go libfuse-dev squashfuse pkg-config`. builds:
- `go build -o /out/snapd ./cmd/snapd`
- `go build -o /out/snap ./cmd/snap`
- `go build -o /out/snapctl ./cmd/snapctl`

that's the whole binary set. `snap-confine` and friends are gone per the deletion list.

### stage 2: seed-builder

ubuntu:24.04. `apt install snapd`. produces a seed:

```sh
snap known --remote model series=16 brand-id=generic model=generic-classic > /tmp/generic-classic.model
snap prepare-image --classic --snap=core24 /tmp/generic-classic.model /seed-out/
```

### stage 3: runtime

ubuntu:24.04 minimal base. `apt install squashfuse fuse libcap2-bin tini ca-certificates`. copies:
- `/out/snapd`, `/out/snap`, `/out/snapctl` from stage 1 -> `/usr/local/bin/`
- `/seed-out/var/lib/snapd/seed/` from stage 2 -> `/var/lib/snapd/seed/`
- `entrypoint.sh`, `demo.sh` -> `/usr/local/bin/`

```dockerfile
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
CMD ["bash"]
```

### entrypoint.sh

```sh
#!/bin/sh
set -e
mkdir -p /run/snapd /snap /var/lib/snapd
/usr/local/bin/snapd &
SNAPD=$!
for i in $(seq 1 30); do
    [ -S /run/snapd.socket ] && break
    sleep 0.5
done
[ -S /run/snapd.socket ] || { echo "snapd failed to come up"; exit 1; }
case "$1" in
    demo) exec /usr/local/bin/demo.sh ;;
    *)    exec "$@" ;;
esac
```

### demo.sh

```sh
#!/bin/sh
set -ex
snap install hello-world
hello-world
snap remove hello-world
```

## phasing

throwaway prototype, single feature branch (`prototype/no-systemd`). suggested order, each step ending in a state where _something_ runs:

### phase 0: branch + chainsaw

```sh
git checkout -b prototype/no-systemd
# run the rm -rf block from "deletion list" above
git add -A && git commit -m "chore!: chainsaw systemd subtrees"
```

`go build ./cmd/snapd ./cmd/snap` will be broken. that's expected.

### phase 1: compile

chase compile errors back through importers. for each broken caller:
- if it's downstream of a deleted subtree, delete it too.
- if it's load-bearing for the install path, stub the missing call.

target: `go build ./cmd/snapd ./cmd/snap` succeeds. tests are not green; many won't compile. that's fine.

useful commands during this phase:

```sh
go build ./... 2>&1 | head -50              # work top-down on errors
grep -rl "github.com/snapcore/snapd/systemd" --include='*.go' .
grep -rl "snapcore/snapd/usersession"        --include='*.go' .
grep -rl "snapcore/snapd/interfaces/systemd" --include='*.go' .
grep -rl "snapcore/snapd/interfaces/dbus"    --include='*.go' .
grep -rln "sd_notify\|sdNotify"              --include='*.go' .
grep -rln "CreateTransientScopeForTracking"  --include='*.go' .
grep -rln "systemctl\|systemd-run"           --include='*.go' .
```

### phase 2: daemon comes up

patch `cmd/snapd/main.go` + `daemon/daemon.go` to drop sd_notify and socket-activation. on a linux box outside any container:

```sh
sudo ./snapd
# in another terminal:
./snap version          # should respond
```

nothing else needs to work yet.

### phase 3: mounts via squashfuse

patch `overlord/snapstate/backend/mountunit.go` per the surgery list. test harness:

```sh
# pull a known-good hello-world.snap once on a working host, copy into the container env
./snap install --dangerous hello-world.snap
ls /snap/hello-world/x1/
mount | grep snap          # should show squashfuse mount
./snap remove --revision=x1 hello-world
mount | grep snap || echo "all unmounted"
```

### phase 4: run path bypass

patch `cmd/snap/cmd_run.go`:

```sh
./snap run hello-world
# expect: "Hello, world!"
```

### phase 5: seed + store install

drop in the `snap prepare-image` seed at `/var/lib/snapd/seed/`. start snapd; watch it consume the seed and mark itself seeded.

```sh
sudo ./snapd  # watch logs for "seeding completed"
./snap list       # should show core24 listed
./snap install hello-world  # full path: store, no --dangerous
```

### phase 6: dockerise

write the `Dockerfile`, `entrypoint.sh`, `demo.sh`. build + run:

```sh
docker build -t snapd-poc .
docker run --rm --device /dev/fuse --cap-add SYS_ADMIN snapd-poc demo
```

### phase 7: cleanup

remove dead code that survived chainsaw + scalpel by accident. trim the binary set (any `cmd/*` that doesn't get invoked by the demo path). update the readme below with anything surprising.

(rocks packaging deferred per q10. once docker works, the surface change is a `rockcraft.yaml` declaring snapd as a pebble service. snapd itself doesn't change.)

## verification

end-to-end (after phase 6):

```sh
docker build -t snapd-poc .
docker run --rm --device /dev/fuse --cap-add SYS_ADMIN snapd-poc demo
```

expected output, roughly:

```
+ snap install hello-world
hello-world ... installed
+ hello-world
Hello World!
+ snap remove hello-world
hello-world removed
```

### manual interactive sanity-check

```sh
docker run --rm -it --device /dev/fuse --cap-add SYS_ADMIN snapd-poc bash
# inside:
snap version
snap list                         # core24 from seed listed
snap install hello-world
ls /snap/hello-world/             # current symlink + revision dir
mount | grep snap                 # squashfuse mount visible
hello-world
snap remove hello-world
mount | grep snap || echo "all unmounted"
```

### tells that the demo is "lying"

if any of these are non-empty, something didn't get removed properly:

```sh
ldd $(which snapd) | grep systemd     # must be empty
ls /etc/systemd/ /run/systemd/ /lib/systemd/ 2>/dev/null   # must not exist
find /var/lib/snapd -name '*.service' -o -name '*.socket' \
                    -o -name '*.timer' -o -name '*.mount'  # must be empty
ps -ef | grep -v -E 'tini|snapd|squashfuse|bash|sh|ps|grep'   # nothing systemd-y
```

## risks / open questions

- **assertion freshness in the seed.** `snap prepare-image` produces a seed with assertions valid at build time. if these expire or the model rotates, the seeded boot fails. acceptable for a throwaway prototype; rebuild the image when broken.
- **store availability.** the demo needs `api.snapcraft.io` reachable. fine in dev, irritating for offline demos. fallback if needed: bake `hello-world.snap` into the seed (turns `snap install` into a no-op, weakens the demo). only do this if network becomes a blocker.
- **phase 1 deletion ripple.** chasing compile errors will surface load-bearing references in unexpected places (e.g. snapstate consults systemd version somewhere obscure). budget extra time for "i thought i deleted this but X still imports it" loops.
- **squashfuse + cgroup interaction.** squashfuse spawns a separate process per mount that lives outside snapd's pid tree (it daemonises). on container shutdown, tini reaps them, but snapd needs to track those pids itself for clean unmounts. lightweight pid-file scheme (described in surgery list) should suffice; flag if it gets weird.
- **rocks follow-up.** v1 is docker only. when porting, the surface change is replacing `tini` + `entrypoint.sh` with a `rockcraft.yaml` declaring snapd as a pebble service. snapd itself doesn't change. cheap; do it after docker works.

## appendix: systemd touchpoints surveyed before planning

a recon agent enumerated every place snapd talks to systemd. summary, by category:

- **the `systemd/` package**: ~80-method `Systemd` interface (`systemd.go`), journal stream (`journal.go`), unit-name escaping (`escape.go`), sd_notify (`sdnotify_linux.go`), sysctl invocation (`sysctl.go`), test fixtures (`systemdtest/`). 275+ go files import it.
- **unit-file generators**: `wrappers/internal/service_unit_gen.go`, `service_socket_gen.go`, `service_timer_gen.go`, `service_slice_gen.go`, `journal_conf_gen.go`. `wrappers/dbus.go` for dbus activation files.
- **mount handling**: `systemd/systemd.go::EnsureMountUnitFileContent` + `assembleMountUnitContent`. `overlord/snapstate/backend/mountunit.go` calls these. initramfs-side `cmd/snap-bootstrap/initramfs_systemd_mount.go` (out of scope for containers).
- **boot path**: `boot/`, `bootloader/`, `cmd/snap-bootstrap/`. uc-only, all out of scope.
- **journal / logging**: `systemd/journal.go::NewJournalStreamFile`, `jctl()`. `daemon/api_apps.go::LogReader()`. `usersession/autostart` redirects stdout/stderr to journald.
- **dbus**: `wrappers/dbus.go` for activation file generation. `interfaces/dbus/`, `interfaces/builtin/dbus.go`. `dbusutil/` package for system/session bus connections (kept; used for non-systemd dbus interactions).
- **cgroups**: `sandbox/cgroup/cgroup.go` for v1/v2 detection (kept). `sandbox/cgroup/tracking.go` for transient-scope-via-systemd-dbus (deleted).
- **own units**: 15 files in `data/systemd/`, 2 in `data/systemd-user/` (deleted).
- **build / packaging**: `packaging/ubuntu-26.04/snapd.install.in:47-49` installs systemd generators (n/a, packaging is out of scope).

key file:line refs for the surgery work:
- `cmd/snap/cmd_run.go:1925` -- the `cgroupCreateTransientScopeForTracking` call to short-circuit.
- `wrappers/dbus.go:42` -- the `SystemdService=` line in dbus activation files (whole file deleted, but worth knowing why it was systemd-shaped).
- `overlord/snapstate/backend/mountunit.go:39-77` -- `addMountUnit` / `removeMountUnit` / `RemoveContainerMountUnits` to rewrite.
- `systemd/systemd.go:71` -- the `daemonReloadLock` mutex; gone with the package, confirm nothing relied on its serialisation.
