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

.PHONY: clean
clean:  ## Remove built binaries
	rm -rf ./bin
