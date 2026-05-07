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

.PHONY: rock-up
rock-up: rock  ## Start the rock container in the background
	-podman rm -f $(ROCK_CONTAINER) >/dev/null 2>&1
	podman run -d --name $(ROCK_CONTAINER) oci-archive:$(ROCK_FILE) sleep infinity

.PHONY: rock-down
rock-down:  ## Stop and remove the rock container
	-podman rm -f $(ROCK_CONTAINER) >/dev/null 2>&1

.PHONY: rock-shell
rock-shell: rock-up  ## Drop into an interactive shell in the rock
	-podman exec -it $(ROCK_CONTAINER) /bin/sh
	podman rm -f $(ROCK_CONTAINER) >/dev/null 2>&1

.PHONY: rock-clean
rock-clean:  ## Remove built rock artefacts
	rm -rf rock/_stage rock/*.rock
	cd rock && rockcraft clean || true

.PHONY: clean
clean:  ## Remove built binaries
	rm -rf ./bin
