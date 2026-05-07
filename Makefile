.SUFFIXES:

GOOS   ?= linux
GOARCH ?= arm64

help:  ## Show this help
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build:  ## Cross-compile the snap binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o ./bin/snap ./cmd/snap

.PHONY: docker
docker:  ## Build the bare snap-poc Docker image
	docker build -t snap-poc .

.PHONY: shell
shell: docker  ## Build and drop into an interactive shell
	docker run --rm -it snap-poc /bin/sh

ROCK_NAME    := snap-rock
ROCK_VERSION := 0.1
ROCK_ARCH    := $(shell dpkg --print-architecture 2>/dev/null || echo $(GOARCH))
ROCK_FILE    := rock/$(ROCK_NAME)_$(ROCK_VERSION)_$(ROCK_ARCH).rock

.PHONY: rock-stage
rock-stage: build  ## Stage the snap binary under rock/_stage/
	mkdir -p rock/_stage
	cp ./bin/snap rock/_stage/

.PHONY: rock
rock: rock-stage  ## Build the snap rock (OCI archive via rockcraft)
	# rockcraft caches part build output and doesn't track _stage/
	# sources, so clean the binaries part to force a re-copy.
	cd rock && rockcraft clean binaries >/dev/null 2>&1 || true
	cd rock && rockcraft pack

ROCK_CONTAINER := snap-rock
ROCK_VOLUME    := snap-rock-state

# rocks default to pebble as pid 1, which would intercept our cli args
# (`pebble enter exec ...` syntax). override --entrypoint so we run
# the snap binary directly. there's no daemon to keep alive anyway.
#
# state goes into a named podman volume mounted at /snap (the
# extracted snaps), /var/snap (per-snap data), /var/lib/snapd (the
# assertion db + cached downloads). without it, every container is
# fresh and you lose installed snaps + the asserts cache between
# invocations.

ROCK_VOLUMES := \
	-v $(ROCK_VOLUME)-snap:/snap \
	-v $(ROCK_VOLUME)-varsnap:/var/snap \
	-v $(ROCK_VOLUME)-snapd:/var/lib/snapd

.PHONY: rock-install
rock-install: rock  ## Run \`snap install <SNAP>\` against the rock
	@if [ -z "$(SNAP)" ]; then echo 'usage: make rock-install SNAP=<name>' >&2; exit 1; fi
	podman run --rm $(ROCK_VOLUMES) --entrypoint /usr/bin/snap \
		oci-archive:$(ROCK_FILE) install $(SNAP)

.PHONY: rock-shell
rock-shell: rock  ## Drop into a shell inside the rock (state persists across runs)
	-podman rm -f $(ROCK_CONTAINER) >/dev/null 2>&1
	@# bootstrap a base snap so /bin/sh exists. idempotent across runs
	@# thanks to the persistent volume + the install path's already-
	@# installed short-circuit.
	podman run --rm $(ROCK_VOLUMES) --entrypoint /usr/bin/snap \
		oci-archive:$(ROCK_FILE) install $(or $(BASE),core22)
	podman run -d --name $(ROCK_CONTAINER) $(ROCK_VOLUMES) \
		--entrypoint /bin/sh oci-archive:$(ROCK_FILE) -c 'sleep infinity'
	-podman exec -it $(ROCK_CONTAINER) /bin/sh
	podman rm -f $(ROCK_CONTAINER) >/dev/null 2>&1

.PHONY: rock-up
rock-up: rock  ## Start the rock detached (state-persisting)
	-podman rm -f $(ROCK_CONTAINER) >/dev/null 2>&1
	podman run -d --name $(ROCK_CONTAINER) $(ROCK_VOLUMES) \
		--entrypoint /usr/bin/snap oci-archive:$(ROCK_FILE) help

.PHONY: rock-down
rock-down:  ## Stop and remove the rock container (volumes kept)
	-podman rm -f $(ROCK_CONTAINER) >/dev/null 2>&1

.PHONY: rock-wipe
rock-wipe: rock-down  ## Stop the rock and delete its state volumes
	-podman volume rm -f $(ROCK_VOLUME)-snap $(ROCK_VOLUME)-varsnap $(ROCK_VOLUME)-snapd >/dev/null 2>&1

.PHONY: rock-clean
rock-clean:  ## Remove built rock artefacts
	rm -rf rock/_stage rock/*.rock
	cd rock && rockcraft clean || true

.PHONY: clean
clean:  ## Remove built binaries
	rm -rf ./bin
