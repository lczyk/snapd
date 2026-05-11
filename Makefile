.SUFFIXES:

GOOS   ?= linux
GOARCH ?= arm64

help:  ## Show this help
	@echo "Available targets:"
	@awk ' \
		function cmp(g, a, b) { \
			if (a == g) return -1; \
			if (b == g) return 1; \
			return a < b ? -1 : (a > b ? 1 : 0) \
		} \
		/^## @help-group:[ \t]*/ { \
			sub(/^## @help-group:[ \t]*/, ""); \
			group = $$0; \
			if (!(group in seen)) { order[n++] = group; seen[group] = 1 } \
			next \
		} \
		/^[a-zA-Z_-]+:.*## / { \
			split($$0, a, /:.*## /); \
			target = a[1]; desc = a[2]; \
			g = group; \
			if (!(g in seen)) { order[n++] = g; seen[g] = 1 } \
			count[g]++; \
			tnames[g, count[g]] = target; \
			descs[g, target] = desc \
		} \
		END { \
			for (i = 0; i < n; i++) { \
				g = order[i]; \
				if (i > 0) print ""; \
				if (g != "") \
					printf "\033[1;33m== %s ==\033[0m\n", g; \
				k = count[g]; \
				for (p = 0; p < k; p++) names[p] = tnames[g, p+1]; \
				for (p = 1; p < k; p++) { \
					cur = names[p]; q = p - 1; \
					while (q >= 0 && cmp(g, names[q], cur) > 0) { names[q+1] = names[q]; q-- } \
					names[q+1] = cur \
				} \
				for (p = 0; p < k; p++) \
					printf "  \033[36m%-16s\033[0m %s\n", names[p], descs[g, names[p]]; \
				delete names \
			} \
		}' $(MAKEFILE_LIST)

## @help-group: snap

.PHONY: snap-build
snap-build:  ## Cross-compile the snap binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o ./bin/snap ./cmd/snap

## @help-group: docker

DOCKER_IMAGE     := snap-poc
DOCKER_CONTAINER := snap-poc
DOCKER_VOLUME    := snap-poc-state

# state goes into named docker volumes mounted at /snap (extracted
# snaps), /var/snap (per-snap data), /var/lib/snapd (assertion db +
# cached downloads). same shape as the rock targets, just docker.

DOCKER_VOLUMES := \
	-v $(DOCKER_VOLUME)-snap:/snap \
	-v $(DOCKER_VOLUME)-varsnap:/var/snap \
	-v $(DOCKER_VOLUME)-snapd:/var/lib/snapd

.PHONY: docker
docker:  ## Build the snap-poc docker image
	docker build -t $(DOCKER_IMAGE) .

.PHONY: docker-install
docker-install: docker  ## Run \`snap install <SNAP>\` in docker
	@if [ -z "$(SNAP)" ]; then echo 'usage: make docker-install SNAP=<name>' >&2; exit 1; fi
	docker run --rm $(DOCKER_VOLUMES) $(DOCKER_IMAGE) install $(SNAP)

.PHONY: docker-shell
docker-shell: docker  ## Drop into a shell in docker (state persists across runs)
	-docker rm -f $(DOCKER_CONTAINER) >/dev/null 2>&1
	@# bootstrap a base snap so /bin/sh exists. idempotent thanks to
	@# the persistent volume + the install path's already-installed
	@# short-circuit.
	docker run --rm $(DOCKER_VOLUMES) $(DOCKER_IMAGE) install $(or $(BASE),core22)
	docker run -d --name $(DOCKER_CONTAINER) $(DOCKER_VOLUMES) \
		--entrypoint /bin/sh $(DOCKER_IMAGE) -c 'sleep infinity'
	-docker exec -it $(DOCKER_CONTAINER) /bin/sh
	docker rm -f $(DOCKER_CONTAINER) >/dev/null 2>&1

.PHONY: docker-up
docker-up: docker  ## Start the docker container detached (state-persisting)
	-docker rm -f $(DOCKER_CONTAINER) >/dev/null 2>&1
	docker run -d --name $(DOCKER_CONTAINER) $(DOCKER_VOLUMES) \
		--entrypoint /bin/sh $(DOCKER_IMAGE) -c 'sleep infinity'

.PHONY: docker-down
docker-down:  ## Stop and remove the docker container (volumes kept)
	-docker rm -f $(DOCKER_CONTAINER) >/dev/null 2>&1

.PHONY: docker-wipe
docker-wipe: docker-down  ## Stop the container and delete its state volumes
	-docker volume rm -f $(DOCKER_VOLUME)-snap $(DOCKER_VOLUME)-varsnap $(DOCKER_VOLUME)-snapd >/dev/null 2>&1

.PHONY: docker-clean
docker-clean: docker-wipe  ## Remove image + state volumes
	-docker rmi -f $(DOCKER_IMAGE) >/dev/null 2>&1

## @help-group: rock

ROCK_NAME    := snap-rock
ROCK_VERSION := 0.1
ROCK_ARCH    := $(shell dpkg --print-architecture 2>/dev/null || echo $(GOARCH))
ROCK_FILE    := rock/$(ROCK_NAME)_$(ROCK_VERSION)_$(ROCK_ARCH).rock

.PHONY: rock-stage
rock-stage: snap-build  ## Stage the snap binary under rock/_stage/
	rm -rf rock/_stage
	mkdir -p rock/_stage/usr/bin
	cp ./bin/snap rock/_stage/usr/bin/snap

.PHONY: rock
rock: rock-stage  ## Build the snap rock (OCI archive via rockcraft)
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

## @help-group:

.PHONY: unit
unit:  ## Run go unit tests across all packages with the race detector
	go test -race ./...

.PHONY: test
test: unit spread  ## Run unit + spread tests

.PHONY: clean
clean: snap-clean docker-clean rock-clean spread-clean  ## Run all *-clean targets

## @help-group: spread

SPREAD_IMAGE := snap-spread-sshd-noble-$(ROCK_ARCH)

.PHONY: spread-image
spread-image:  ## Build the sshd image used by the spread adhoc backend
	@if [ -z "$$(docker images -q $(SPREAD_IMAGE) 2>/dev/null)" ]; then \
		docker build -t $(SPREAD_IMAGE) \
			-f tests/spread/images/Dockerfile.sshd-noble \
			--platform linux/$(ROCK_ARCH) .; \
	else \
		echo 'image $(SPREAD_IMAGE) already built'; \
	fi

# spread's filter rejects trailing slashes ("nothing matches provider
# filter"). pass the bare task path without the trailing /.
LEAN_TASKS := \
	tests/spread/integration/install \
	tests/spread/integration/channel \
	tests/spread/integration/sideload \
	tests/spread/integration/list-remove

.PHONY: spread
spread: snap-build spread-image  ## Run the lean spread tasks (install + channel + sideload + list-remove)
	spread $(LEAN_TASKS)

.PHONY: spread-extended
spread-extended: snap-build spread-image  ## Run the extended install task
	spread tests/spread/integration/install-extended

.PHONY: spread-debug
spread-debug: snap-build spread-image  ## Run spread w/ -debug -v (drops to shell on failure)
	spread -debug -v $(LEAN_TASKS)

.PHONY: spread-list
spread-list:  ## List all discovered spread tasks
	spread -list

.PHONY: spread-clean
spread-clean:  ## Remove spread containers, image, blob cache, and worker counter
	-docker ps -a --filter "name=snap-spread-" --format "{{.ID}}" | xargs -r docker rm -f
	-docker images --filter=reference='snap-spread-sshd-*' --format "{{.ID}}" | xargs -r docker rmi -f
	rm -rf tests/spread/.cache
	rm -f .spread-worker-num .spread-reuse.yaml

## @help-group: snap

.PHONY: snap-clean
snap-clean:  ## Remove built binaries
	rm -rf ./bin
