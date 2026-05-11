# single-binary snap prototype: a static go binary, nothing else.
# install / run snaps in a container, with the container as the
# security boundary. base snaps are downloaded into /snap/<base>/
# on first install and provide all the libs the snap apps need.

# go.mod requires 1.24, ubuntu:24.04 ships 1.22 -- use the official
# golang image instead of pulling go from apt.
FROM golang:1.24 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/snap ./cmd/snap

# runtime: ubuntu:24.04 provides /bin/sh, libc, and standard tools so
# classic snaps (dynamically-linked go, python, etc.) resolve their
# interpreter + deps against the host. base snaps are still needed for
# snaps that bundle their own userland; the snap binary wires them up
# only when the host doesn't already own /lib/<triplet>.
FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/snap /usr/bin/snap
ENV PATH=/snap/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENTRYPOINT ["/usr/bin/snap"]
