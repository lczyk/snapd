module github.com/snapcore/snapd

go 1.24

// maze.io/x/crypto/afis imported by github.com/snapcore/secboot/tpm2
replace maze.io/x/crypto => github.com/snapcore/maze.io-x-crypto v0.0.0-20190131090603-9b94c9afe066

require (
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf
	github.com/godbus/dbus/v5 v5.1.0
	github.com/juju/ratelimit v1.0.1
	github.com/mattn/go-runewidth v0.0.15
	github.com/mvo5/goconfigparser v0.0.0-20231016112547-05bd887f05e1
	golang.org/x/crypto v0.23.0
	golang.org/x/net v0.21.0 // indirect
	golang.org/x/sync v0.8.0
	golang.org/x/sys v0.21.0
	gopkg.in/macaroon.v1 v1.0.0
	gopkg.in/retry.v1 v1.0.3
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/klauspost/compress v1.18.6
	github.com/pierrec/lz4/v4 v4.1.26
	github.com/rasky/go-lzo v0.0.0-20200203143853-96a758eda86e
	github.com/ulikunitz/xz v0.5.15
)

require (
	github.com/google/go-cmp v0.5.8 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	golang.org/x/term v0.20.0 // indirect
)
