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

# runtime: scratch + the snap binary + ca certs (so the store TLS
# verifies) + a couple of empty dirs (/tmp and /var/lib/snapd) so
# bind-mounting host files / volumes for sideload + state-persist
# works without docker auto-creating a directory at the mount path.
# everything else (bash, libc, /lib/ld-linux-*, ...) gets pulled in
# via the first base-snap install at runtime.
FROM scratch
COPY --from=builder /out/snap /usr/bin/snap
COPY --from=builder /etc/ssl/certs /etc/ssl/certs
COPY --from=builder /tmp /tmp
COPY --from=builder /var/lib /var/lib
ENV PATH=/snap/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENTRYPOINT ["/usr/bin/snap"]
