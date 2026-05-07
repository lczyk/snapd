# no-systemd prototype: snapd in a bare docker container
# squashfs extraction is now built into snapd (native Go reader) -- no unsquashfs needed.

# stage 1: build snapd binaries
FROM ubuntu:26.04 AS builder
RUN apt-get update && apt-get install -y golang-go pkg-config ca-certificates git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/snapd ./cmd/snapd \
 && CGO_ENABLED=0 go build -o /out/snap ./cmd/snap \
 && CGO_ENABLED=0 go build -o /out/snapctl ./cmd/snapctl

# stage 2: file collector
FROM ubuntu:26.04 AS collector
RUN apt-get update && apt-get install -y ca-certificates busybox tini
RUN mkdir -p /out/bin /out/etc/ssl /out/usr/local/bin /out/usr/bin

# snapd binaries (static Go -- CGO_ENABLED=0)
COPY --from=builder /out/snapd /out/usr/local/bin/
COPY --from=builder /out/snap  /out/usr/bin/
COPY --from=builder /out/snapctl /out/usr/local/bin/
RUN ln -s /usr/bin/snap /out/usr/local/bin/snap

# busybox provides sh, mkdir, sleep, cat, ls, test, rm, tar, gzip
RUN cp /bin/busybox /out/bin/ \
 && for cmd in sh mkdir sleep cat ls test rm tar gzip wget echo printf; do \
      ln -s busybox /out/bin/$cmd; \
    done

# tini (init process for signal handling)
RUN cp /usr/bin/tini /out/usr/bin/

# shared libraries -- copy linker and libc for dynamically-linked tools (busybox, tini)
# also set up the multiarch path so that snaps with dynamic binaries can run
RUN linker=$(ldd /bin/busybox | grep ld-linux | awk '{print $1}') \
 && libdir=$(dirname "$linker") \
 && mkdir -p "/out$libdir" "/out/lib" \
 && cp "$linker" "/out$libdir/" \
 && cp "$linker" /out/lib/ \
 && for bin in /bin/busybox /usr/bin/tini; do \
      ldd "$bin" | grep '=>' | awk '{print $3}' | sort -u | while read -r lib; do \
        [ -f "$lib" ] && cp -n "$lib" "/out$libdir/"; \
        [ -f "$lib" ] && cp -n "$lib" /out/lib/; \
      done; \
    done

# CA certificates (for HTTPS to store)
RUN cp -r /etc/ssl/certs /out/etc/ssl/

# /etc/passwd (so we have a username)
RUN echo 'root:x:0:0:root:/root:/bin/sh' > /out/etc/passwd \
 && echo 'root:x:0:' > /out/etc/group \
 && echo 'root:*:20000:0:99999:7:::' > /out/etc/shadow \
 && echo 'hosts: files dns' > /out/etc/nsswitch.conf \
 && mkdir -p /out/tmp /out/root /out/run

# seed
COPY entrypoint.sh /out/usr/local/bin/
COPY demo.sh /out/usr/local/bin/
RUN chmod +x /out/usr/local/bin/entrypoint.sh /out/usr/local/bin/demo.sh

# stage 3: bare runtime
FROM scratch
COPY --from=collector /out/ /
ENV PATH=/snap/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
CMD ["/bin/sh"]
