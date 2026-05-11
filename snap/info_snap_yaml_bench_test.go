package snap

import (
	"testing"

	"gopkg.in/yaml.v2"
)

const snapYamlMinimal = `name: test-minimal
version: "1.0"
summary: minimal test snap with no apps
base: none
`

const snapYamlTypical = `name: hello-world
version: "2.10"
summary: Hello world example
description: |
  A longer description for the hello-world snap that demonstrates
  a typical snap.yaml with multiple lines.
type: app
base: core24
confinement: strict
grade: stable
architectures:
  - amd64
  - arm64
apps:
  hello:
    command: bin/hello
    plugs:
      - network
      - home
  goodbye:
    command: bin/goodbye
    daemon: simple
    daemon-scope: system
    restart-condition: on-failure
    stop-timeout: 10s
    plugs:
      - network-bind
hooks:
  configure:
    plugs:
      - network
plugs:
  network:
    interface: network
  network-bind:
    interface: network-bind
  home:
    interface: home
slots:
  hello-slot:
    interface: hello
`

const snapYamlComplex = `name: complex-app
version: "3.14.159-26535-g1234567"
summary: A complex snap with many apps, hooks, plugs, and slots
description: |
  This is a complex snap.yaml that exercises all the parsing paths.
  It has multiple apps, hooks, plugs, slots, layouts, and system usernames.
type: app
base: core24
confinement: strict
grade: stable
license: GPL-3.0
provenance: canonic
epoch: "1*"
architectures:
  - amd64
  - arm64
  - armhf
  - i386
assumes:
  - snapd2.58
  - command-chain
environment:
  GLOBAL_VAR: value
  ANOTHER_VAR: another
apps:
  server:
    command: bin/server
    daemon: simple
    daemon-scope: system
    restart-condition: on-failure
    stop-timeout: 30s
    start-timeout: 60s
    watchdog-timeout: 120s
    stop-command: bin/stop-server
    reload-command: bin/reload-server
    post-stop-command: bin/post-stop-server
    stop-mode: sigterm
    refresh-mode: endure
    install-mode: enable
    before:
      - worker
    after:
      - database
    plugs:
      - network
      - network-bind
      - home
      - log-observe
      - mount-observe
      - system-observe
      - hardware-observe
      - process-control
    slots:
      - server-slot
    sockets:
      control:
        listen-stream: $SNAP_DATA/control.sock
        socket-mode: 0660
    environment:
      APP_VAR: app-value
      DEBUG: "true"
    completer: completions/server
    common-id: com.example.server
    success-exit-status:
      - "143"
    timer: "mon,10:00"
    autostart: "server-env"
  worker:
    command: bin/worker
    daemon: simple
    daemon-scope: system
    restart-condition: always
    restart-delay: 5s
    stop-timeout: 10s
    after:
      - server
    plugs:
      - network
      - home
    environment:
      WORKER_VAR: worker-value
  database:
    command: bin/database
    daemon: notify
    daemon-scope: system
    restart-condition: never
    stop-timeout: 60s
    plugs:
      - network
      - network-bind
    slots:
      - database-slot
    bus-name: com.example.database
    activates-on:
      - database-slot
  cli:
    command: bin/cli
    completer: completions/cli
    plugs:
      - home
    aliases:
      - cli-alias
hooks:
  configure:
    plugs:
      - network
  install:
  remove:
  check-health:
    plugs:
      - network
      - system-observe
  default-configure:
plugs:
  network:
    interface: network
  network-bind:
    interface: network-bind
  home:
    interface: home
  log-observe:
    interface: log-observe
  mount-observe:
    interface: mount-observe
  system-observe:
    interface: system-observe
  hardware-observe:
    interface: hardware-observe
  process-control:
    interface: process-control
slots:
  server-slot:
    interface: hello
    label: Server interface
  database-slot:
    interface: dbus
    bus: system
    name: com.example.database
layout:
  /usr/share/server:
    bind: $SNAP/usr/share/server
  /etc/server.conf:
    symlink: $SNAP_DATA/etc/server.conf
  /var/cache/complex:
    type: tmpfs
    mode: 1777
system-usernames:
  snap_daemon:
    scope: shared
  snap_complex:
    scope: shared
    attrib1: value1
links:
  contact:
    - mailto:hello@example.com
  website:
    - https://example.com
  donation:
    - https://example.com/donate
  issues:
    - https://example.com/issues
components:
  my-component:
    type: standard
    summary: Standard component
    description: A standard component for the complex snap
    hooks:
      install:
      pre-refresh:
`

func BenchmarkInfoFromSnapYaml(b *testing.B) {
	b.Run("minimal", func(b *testing.B) {
		yamlData := []byte(snapYamlMinimal)
		b.ResetTimer()
		for b.Loop() {
			InfoFromSnapYaml(yamlData)
		}
	})
	b.Run("typical", func(b *testing.B) {
		yamlData := []byte(snapYamlTypical)
		b.ResetTimer()
		for b.Loop() {
			InfoFromSnapYaml(yamlData)
		}
	})
	b.Run("complex", func(b *testing.B) {
		yamlData := []byte(snapYamlComplex)
		b.ResetTimer()
		for b.Loop() {
			InfoFromSnapYaml(yamlData)
		}
	})
}

func BenchmarkInfoFromSnapYamlThenValidate(b *testing.B) {
	b.Run("typical", func(b *testing.B) {
		yamlData := []byte(snapYamlTypical)
		b.ResetTimer()
		for b.Loop() {
			info, err := InfoFromSnapYaml(yamlData)
			if err != nil {
				b.Fatal(err)
			}
			if err := Validate(info); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("complex", func(b *testing.B) {
		yamlData := []byte(snapYamlComplex)
		b.ResetTimer()
		for b.Loop() {
			info, err := InfoFromSnapYaml(yamlData)
			if err != nil {
				b.Fatal(err)
			}
			if err := Validate(info); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkSanitizePlugsSlotsCalled(b *testing.B) {
	// The old SanitizePlugsSlots is nil, we set a noop
	old := SanitizePlugsSlots
	SanitizePlugsSlots = func(snapInfo *Info) {}
	defer func() { SanitizePlugsSlots = old }()

	b.Run("typical", func(b *testing.B) {
		yamlData := []byte(snapYamlTypical)
		b.ResetTimer()
		for b.Loop() {
			var y snapYaml
			if err := yaml.Unmarshal(yamlData, &y); err != nil {
				b.Fatal(err)
			}
			infoFromSnapYaml(yamlData, new(scopedTracker))
		}
	})
	b.Run("complex", func(b *testing.B) {
		yamlData := []byte(snapYamlComplex)
		b.ResetTimer()
		for b.Loop() {
			var y snapYaml
			if err := yaml.Unmarshal(yamlData, &y); err != nil {
				b.Fatal(err)
			}
			infoFromSnapYaml(yamlData, new(scopedTracker))
		}
	})
}
