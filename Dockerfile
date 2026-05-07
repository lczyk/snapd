# single-binary snap prototype: a static go binary, nothing else.
# install / run snaps in a container, with the container as the
# security boundary. base snaps are downloaded into /snap/<base>/
# on first install and provide all the libs the snap apps need.

FROM ubuntu:24.04 AS builder
RUN apt-get update && apt-get install -y --no-install-recommends \
    golang-go ca-certificates \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/snap ./cmd/snap

# runtime: scratch + the snap binary + ca certs (so the store TLS
# verifies). everything else (bash, libc, /lib/ld-linux-*, ...) gets
# pulled in via the first base-snap install at runtime.
FROM scratch
COPY --from=builder /out/snap /usr/bin/snap
COPY --from=builder /etc/ssl/certs /etc/ssl/certs
ENV PATH=/snap/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENTRYPOINT ["/usr/bin/snap"]
