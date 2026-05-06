# no-systemd prototype: snapd in a bare docker container

# stage 1: build snapd binaries
FROM ubuntu:24.04 AS builder
RUN apt-get update && apt-get install -y golang-go pkg-config ca-certificates git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/snapd ./cmd/snapd \
 && go build -o /out/snap ./cmd/snap \
 && go build -o /out/snapctl ./cmd/snapctl

# stage 2: file collector
FROM ubuntu:24.04 AS collector
RUN apt-get update && apt-get install -y squashfs-tools ca-certificates tini dash
RUN mkdir -p /out/bin /out/lib /out/etc/ssl /out/usr/local/bin /out/usr/bin

# snapd binaries (static)
COPY --from=builder /out/snapd /out/usr/local/bin/
COPY --from=builder /out/snap  /out/usr/bin/
COPY --from=builder /out/snapctl /out/usr/local/bin/
RUN ln -s /usr/bin/snap /out/usr/local/bin/snap

# shell + basic utils (copy binary + shared libs)
RUN cp /bin/dash /bin/mkdir /bin/sleep /bin/cat /bin/ls /bin/test /bin/rm /bin/tar /bin/gzip /out/bin/ \
 && ln -s dash /out/bin/sh

# unsquashfs + tini
RUN cp /usr/bin/unsquashfs /usr/bin/tini /out/usr/bin/

# shared libraries - discover paths dynamically (works on arm64 and amd64)
RUN linker=$(ldd /bin/sh | grep ld-linux | awk '{print $1}') \
 && libdir=$(ldd /bin/sh | grep '=>' | head -1 | awk '{print $3}' | xargs dirname) \
 && echo "linker: $linker, libdir: $libdir" \
 && mkdir -p "/out$libdir" \
 && cp -a "$libdir"/*.so* "/out$libdir/" \
 && mkdir -p /out/lib \
 && cp "$linker" /out/lib/

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
