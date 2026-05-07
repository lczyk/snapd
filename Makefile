.SUFFIXES:

GOOS   ?= linux
GOARCH ?= arm64

help:  ## Show this help
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build:  ## Cross-compile snapd, snap, snapctl
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o ./bin/snapd ./cmd/snapd
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o ./bin/snap ./cmd/snap
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o ./bin/snapctl ./cmd/snapctl

.PHONY: docker
docker:  ## Build the bare snapd-poc Docker image
	docker build -t snapd-poc .

.PHONY: demo
demo: docker  ## Build and run the canned demo
	docker run --rm snapd-poc demo

.PHONY: shell
shell: docker  ## Build and drop into an interactive shell
	docker run --rm -it snapd-poc /bin/sh

.PHONY: tree
tree: docker  ## List the bare container filesystem (tar tv)
	@docker create --name snapd-poc-tree snapd-poc >/dev/null; \
	docker export snapd-poc-tree | tar tv | sort -k6; \
	docker rm snapd-poc-tree >/dev/null

.PHONY: image-size
image-size: docker  ## Show the Docker image size
	@docker images snapd-poc --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"

ROCK_NAME    := snapd-rock
ROCK_VERSION := 0.1
ROCK_ARCH    := $(shell dpkg --print-architecture 2>/dev/null || echo $(GOARCH))
ROCK_FILE    := rock/$(ROCK_NAME)_$(ROCK_VERSION)_$(ROCK_ARCH).rock

.PHONY: rock-stage
rock-stage: build  ## Stage pre-built bins + scripts under rock/_stage/
	mkdir -p rock/_stage
	cp ./bin/snapd ./bin/snap ./bin/snapctl demo.sh rock/snapd-start.sh rock/_stage/
	mv rock/_stage/snapd-start.sh rock/_stage/snapd-start
	chmod +x rock/_stage/snapd-start rock/_stage/demo.sh

.PHONY: rock
rock: rock-stage  ## Build the snapd rock (OCI archive via rockcraft)
	# rockcraft caches part build output and doesn't track _stage/ sources,
	# so clean the binaries part to force a re-copy from the staged files.
	cd rock && rockcraft clean binaries >/dev/null 2>&1 || true
	cd rock && rockcraft pack

ROCK_CONTAINER := snapd-rock

# rock workflow: pebble + the snapd service start when the container is
# launched (`podman run -d`), interactive work happens via `podman exec`.
# same model as fluentd-rock. ROCK_CONTAINER names the container so we
# can reliably exec into / clean up.
.PHONY: rock-up
rock-up: rock  ## Start the rock container in the background
	-podman rm -f $(ROCK_CONTAINER) >/dev/null 2>&1
	podman run -d --name $(ROCK_CONTAINER) oci-archive:$(ROCK_FILE)

.PHONY: rock-down
rock-down:  ## Stop and remove the rock container
	-podman rm -f $(ROCK_CONTAINER) >/dev/null 2>&1

.PHONY: rock-demo
rock-demo: rock-up  ## Run the canned demo against the running rock
	podman exec $(ROCK_CONTAINER) /usr/local/bin/demo.sh; \
		rc=$$?; podman rm -f $(ROCK_CONTAINER) >/dev/null 2>&1; exit $$rc

.PHONY: rock-shell
rock-shell: rock-up  ## Drop into an interactive shell in the rock
	-podman exec -it $(ROCK_CONTAINER) /usr/bin/bash
	podman rm -f $(ROCK_CONTAINER) >/dev/null 2>&1

.PHONY: rock-clean
rock-clean:  ## Remove built rock artefacts
	rm -rf rock/_stage rock/*.rock
	cd rock && rockcraft clean || true

.PHONY: clean
clean:  ## Remove built binaries
	rm -rf ./bin
