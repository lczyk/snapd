# spread test plan: single-binary snap prototype

end-to-end integration tests for the chainsawed `snap` binary, driven by
[spread](https://github.com/canonical/spread) over its `adhoc` backend
against docker containers. tests exercise the *binary*, not the demo
container -- spinning up a permissive ubuntu host with the binary copied
in is fine, the production scratch image is not what's being tested.

shape modelled on [spread-bread](https://github.com/lczyk/spread-bread)
(allocate / discard pattern, parallel-worker tricks) and on
[chisel-releases](https://github.com/canonical/chisel-releases) test
layout (one task dir per feature, `variants:` for angles, `test_<variant>.sh`
per angle).

## scope

**in**: `snap install` (store + sideload), `snap run` via `/snap/bin/<x>`
shim, `snap refresh` w/ + w/out `--channel`, `snap install --channel=<x>`,
`.channel` persistence across refreshes, `snap list`, `snap remove`, base
snap auto-pull, `wireBaseFs` + `LD_LIBRARY_PATH` on host paths.

**out**: testing the production scratch image (no sshd; `make docker-shell`
stays the manual smoke test for that). amd64 (designed-portable but
unconfigured for v1). gui snaps (out of scope for the binary itself).
ci -- local `make spread` only, ci added later.

## design decisions (and why)

ten forks were settled in conversation. the short version:

| q | decision | reasoning |
|---|---|---|
| 1. test container shape | ubuntu:24.04 + openssh-server + binary copied in | spread needs ssh; production image is scratch w/out sshd. integration tests target the binary, not the image. |
| 2. state isolation | paranoid wipe between every task | removes order-dependence + masking. cost paid back by (3). |
| 3. download cost | bind-mount host blob cache at `/var/lib/snapd/snaps/`, gitignored | wipe spares the cache so re-extracts are cheap, re-downloads avoided. atomic-rename in `store.Download` makes concurrent workers safe. |
| 4. assertion depth | per-snap natural assertion (smoke-grade or i/o roundtrip) | install pipeline is what we own; deep-testing jq is a jq concern. |
| 5. dir layout | chisel-shape, one task dir per feature, variants for angles | matches the reference. adding a new snap = one variant + one bash script. |
| 6. scope tier | tiered. lean default (4 install variants + channel + sideload + list-remove) + opt-in extended (7 more snaps) | fast default loop; full coverage one command away. |
| 7. helpers | none for v1 | three install tests don't earn an abstraction. add when repetition actually shows up. |
| 8. failure debugging | `make spread` (clean) + `make spread-debug` (`-debug -v`) | covers ~95% of "why did this break" without target sprawl. |
| 9. arch | arm64 only in v1, designed portable | this machine is arm64; amd64 under qemu is slow + unrepresentative. adding amd64 later is two lines. |
| 10. ci | local-only (`make spread`) | prototype branch, ci is premature. |

## directory layout

```
spread.yaml                                     # project root
tests/spread/
  images/Dockerfile.sshd-noble                  # ubuntu:24.04 + openssh-server
  scripts/allocate.sh                           # docker run -d, print ADDRESS
  scripts/discard.sh                            # find by IP, docker rm -f
  install/
    task.yaml                                   # variants: [hello-world, jq, htop, btop]
    test_hello-world.sh
    test_jq.sh
    test_htop.sh
    test_btop.sh
  install-extended/
    task.yaml                                   # variants: [yq, tree, hello, ngrok, terraform, kubectl, helm]
    test_yq.sh
    test_tree.sh
    test_hello.sh
    test_ngrok.sh
    test_terraform.sh
    test_kubectl.sh
    test_helm.sh
  channel/
    task.yaml                                   # variants: [install-edge, refresh-stays, refresh-switches]
    test_install-edge.sh
    test_refresh-stays.sh
    test_refresh-switches.sh
  sideload/
    task.yaml
    test.sh
  list-remove/
    task.yaml
    test.sh
  .cache/snaps/                                 # gitignored, host-side blob cache
```

## the spread.yaml in outline

- `project: snap`
- `path: /snap-test` (production `/snap` is reserved for installed snaps in the test container)
- `exclude: [.git, rock, tests/spread/.cache, docs]` -- repo is ~300 MB before exclusion, ~40 MB after; sync stays fast
- one backend `custom-docker`, type `adhoc`, scripts at `tests/spread/scripts/`
- one system `noble-arm64`, `username: root`, `password: snap-test`, `workers: 4`
- project-level `prepare:` -- `install -m 0755 $PROJECT_PATH/bin/snap /usr/bin/snap`
- project-level `restore-each:` -- `rm -rf /snap/* /var/snap/* /var/lib/snapd/{state*,assertions}`
- four suites: `tests/spread/install/`, `install-extended/`, `channel/`, `sideload/`, `list-remove/`

## the allocate / discard pattern

inherited verbatim from spread-bread (with prefix renamed `snap-spread-`):

- **allocate.sh**: parses `$SPREAD_SYSTEM` for flavour + arch; uses `flock` on a counter file to assign unique container names per worker (otherwise parallel workers collide); `docker run -d --platform linux/$arch ... -v $cache:/var/lib/snapd/snaps`; waits for `pgrep sshd`; prints `ADDRESS <bridge-ip>`.
- **discard.sh**: spread only hands `SPREAD_SYSTEM_ADDRESS` to discard, not the container name -- so the script iterates all bridge containers, inspects each for matching ip, removes the match. without this, parallel workers can't be torn down. this *is* the hack the user flagged.

## task shape

each `task.yaml` is minimal:

```yaml
summary: install + smoke-test the lean snap set
variants:
  - hello-world
  - jq
  - htop
  - btop
execute: bash ./test_${SPREAD_VARIANT}.sh
```

each `test_<variant>.sh` is self-contained bash, no shared lib:

```bash
#!/usr/bin/env bash
set -eux
snap install jq
test -e /snap/jq/current/meta/snap.yaml
echo '{"a":1}' | snap run jq -r .a | grep -q '^1$'
```

richer assertions where they fit naturally; `--version` smoke where they
don't (htop, ngrok, terraform, etc).

## makefile additions

```
make spread-image       # docker build the sshd image (arm64 today)
make spread             # build binary + image, run lean suites
make spread-extended    # run the extended install suite
make spread-debug       # spread -debug -v, drops to shell on failure
make spread-list        # spread -list
make spread-clean       # rm containers + image + cache + .spread-worker-num
```

## .gitignore additions

```
tests/spread/.cache/
.spread-worker-num
.spread-reuse.yaml      # already covered by .spread-reuse*.yaml above
```

## v2 notes

- amd64: add `noble-amd64` to `systems:` + one Dockerfile build line. allocate/discard already arch-aware.
- ci: when test set stabilises, drop in `.github/workflows/spread.yml` running `make spread` on push.
- helpers: introduce `tests/spread/lib/` only if duplication crosses ~3 tests for the same shape.
- additional install variants: extend `install-extended/` as new working snaps come up.
