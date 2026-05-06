# no-systemd prototype: snapd in a vanilla docker container

# stage 1: build snapd binaries
FROM ubuntu:24.04 AS builder
RUN apt-get update && apt-get install -y golang-go libfuse3-dev squashfuse pkg-config ca-certificates git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/snapd ./cmd/snapd \
 && go build -o /out/snap ./cmd/snap \
 && go build -o /out/snapctl ./cmd/snapctl

# stage 2: seed builder
FROM ubuntu:24.04 AS seed-builder
RUN apt-get update && apt-get install -y snapd ca-certificates
RUN mkdir -p /seed-out/var/lib/snapd/seed \
 && snap known --remote model series=16 brand-id=generic model=generic-classic > /tmp/generic-classic.model \
 && (snap prepare-image --classic --arch amd64 /tmp/generic-classic.model /seed-out/ || true) \
 && touch /seed-out/var/lib/snapd/seed/.seeded 2>/dev/null || (mkdir -p /seed-out/var/lib/snapd/seed && touch /seed-out/var/lib/snapd/seed/.empty)

# stage 3: runtime
FROM ubuntu:24.04
RUN apt-get update && apt-get install -y squashfuse fuse libcap2-bin ca-certificates tini squashfs-tools && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/snapd /usr/local/bin/
COPY --from=builder /out/snap /usr/bin/
COPY --from=builder /out/snapctl /usr/local/bin/
RUN ln -sf /usr/bin/snap /usr/local/bin/snap
COPY --from=seed-builder /seed-out/var/lib/snapd/seed /var/lib/snapd/seed/

COPY entrypoint.sh /usr/local/bin/
COPY demo.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/demo.sh

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
CMD ["bash"]
