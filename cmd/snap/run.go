// `snap run <name>[.<app>]` -- exec an installed snap's app with the
// right env. classic-only: no mount namespace, no apparmor, no
// seccomp. just env setup + exec.
//
// the env mirrors what snap-exec normally does:
//   SNAP, SNAP_DATA, SNAP_COMMON, SNAP_NAME, SNAP_VERSION, SNAP_REVISION,
//   SNAP_ARCH, SNAP_USER_DATA, SNAP_USER_COMMON, HOME (for classic this
//   one stays as the host's $HOME), plus LD_LIBRARY_PATH set to the
//   base snap's lib dirs (so dynamically-linked snap binaries resolve
//   their libc / libssl / libldap / ...).
//
// the snap.yaml's app-level `environment:` block is applied last, with
// $VAR expansion -- this is how snaps point LD_LIBRARY_PATH at their
// own bundled libs ($SNAP/usr/local/lib etc).

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/snapcore/snapd/arch"
	"github.com/snapcore/snapd/snap"
)

func cmdRun(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("run needs a snap[.app] target")
	}
	target := args[0]
	rest := args[1:]

	snapName, appName := splitTarget(target)

	mountDir := filepath.Join("/snap", snapName, "current")
	if _, err := os.Stat(mountDir); err != nil {
		return fmt.Errorf("snap %q is not installed", snapName)
	}

	yamlBytes, err := os.ReadFile(filepath.Join(mountDir, "meta", "snap.yaml"))
	if err != nil {
		return fmt.Errorf("read snap.yaml: %w", err)
	}
	info, err := snap.InfoFromSnapYaml(yamlBytes)
	if err != nil {
		return fmt.Errorf("parse snap.yaml: %w", err)
	}

	// resolve revision from the symlink target so SNAP / SNAP_REVISION
	// are concrete paths, not "current".
	revStr, err := os.Readlink(mountDir)
	if err != nil {
		return fmt.Errorf("readlink current: %w", err)
	}
	revStr = filepath.Base(revStr)
	concreteMount := filepath.Join("/snap", snapName, revStr)

	if appName == "" {
		// pick a single sensible default if there's only one app
		switch len(info.Apps) {
		case 0:
			return fmt.Errorf("snap %q has no apps", snapName)
		case 1:
			for n := range info.Apps {
				appName = n
			}
		default:
			if _, ok := info.Apps[snapName]; ok {
				appName = snapName
			} else {
				return fmt.Errorf("snap %q has multiple apps; specify <name>.<app>", snapName)
			}
		}
	}
	app, ok := info.Apps[appName]
	if !ok {
		return fmt.Errorf("app %q not found in snap %q", appName, snapName)
	}

	cmdPath := filepath.Join(concreteMount, app.Command)
	finalArgv := append([]string{cmdPath}, rest...)

	// command-chain (wrapper.sh etc.) runs ahead of the app's command
	if len(app.CommandChain) > 0 {
		chain := make([]string, 0, len(app.CommandChain))
		for _, e := range app.CommandChain {
			chain = append(chain, filepath.Join(concreteMount, e))
		}
		finalArgv = append(chain, finalArgv...)
	}

	envMap := buildRunEnv(info, app, concreteMount, snapName, revStr)
	envSlice := mapToEnv(envMap)

	// lazy-launch any declared daemon supervisors that aren't running.
	// covers both `snap run foo` and the /snap/bin/foo shim path, so
	// daemons come up automatically after a container restart.
	ensureDaemonsRunning(snapName)

	if err := syscall.Exec(finalArgv[0], finalArgv, envSlice); err != nil {
		return fmt.Errorf("exec %s: %w", finalArgv[0], err)
	}
	return nil
}

func splitTarget(s string) (snapName, appName string) {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// buildRunEnv computes the full env passed to the snap app. order:
//
//  1. inherit os.Environ() (HOME, USER, TERM, ...)
//  2. overlay SNAP_* and our LD_LIBRARY_PATH
//  3. overlay snap.yaml top-level + app-level `environment:` blocks
//     with $VAR expansion against the env so far
//
// LD_LIBRARY_PATH points at the base snap's lib dirs so the snap's
// dynamically-linked binaries resolve their libc / libssl / etc.
// the snap usually appends its own lib dir on top in step 3.
func buildRunEnv(info *snap.Info, app *snap.AppInfo, mountDir, snapName, rev string) map[string]string {
	env := envSliceToMap(os.Environ())

	env["SNAP"] = mountDir
	env["SNAP_NAME"] = snapName
	env["SNAP_INSTANCE_NAME"] = snapName
	env["SNAP_VERSION"] = info.Version
	env["SNAP_REVISION"] = rev
	env["SNAP_ARCH"] = arch.DpkgArchitecture()
	env["SNAP_DATA"] = filepath.Join("/var/snap", snapName, rev)
	env["SNAP_COMMON"] = filepath.Join("/var/snap", snapName, "common")
	if home := env["HOME"]; home != "" {
		env["SNAP_USER_DATA"] = filepath.Join(home, "snap", snapName, rev)
		env["SNAP_USER_COMMON"] = filepath.Join(home, "snap", snapName, "common")
		env["SNAP_REAL_HOME"] = home
	}

	// PATH + LD_LIBRARY_PATH point at the base snap's libs and bins so
	// dynamically-linked snap binaries resolve their libc / libssl /
	// etc., and shebangs find /bin/sh + standard utilities. but: if the
	// host owns /lib/<triplet> as a real dir (= ubuntu container running
	// the spread tests, not the production scratch image where wireBaseFs
	// symlinks it to the base snap), skip both overrides. otherwise the
	// host's /bin/sh gets paired with core22's libc.so.6 and crashes on
	// glibc-private symbol mismatch. trust the host's own userland.
	if base := info.Base; base != "" && base != "none" && base != "bare" && !hostHasOwnUserland() {
		baseRoot := filepath.Join("/snap", base, "current")
		env["LD_LIBRARY_PATH"] = strings.Join([]string{
			filepath.Join(baseRoot, "usr/lib", multiarchTriplet()),
			filepath.Join(baseRoot, "lib", multiarchTriplet()),
			filepath.Join(baseRoot, "usr/lib"),
			filepath.Join(baseRoot, "lib"),
			env["LD_LIBRARY_PATH"],
		}, ":")
		basePath := strings.Join([]string{
			filepath.Join(baseRoot, "usr/local/sbin"),
			filepath.Join(baseRoot, "usr/local/bin"),
			filepath.Join(baseRoot, "usr/sbin"),
			filepath.Join(baseRoot, "usr/bin"),
			filepath.Join(baseRoot, "sbin"),
			filepath.Join(baseRoot, "bin"),
		}, ":")
		if existing := env["PATH"]; existing != "" {
			env["PATH"] = basePath + ":" + existing
		} else {
			env["PATH"] = basePath
		}
	}

	// snap.yaml top-level env, then per-app env, with $VAR expansion
	// against the env built so far. snap-exec does this via an
	// EnvChain helper; we inline a simpler version.
	for _, name := range info.Environment.Keys() {
		env[name] = expand(info.Environment.Get(name), env)
	}
	if app != nil {
		for _, name := range app.Environment.Keys() {
			env[name] = expand(app.Environment.Get(name), env)
		}
	}
	return env
}

// expand resolves $VAR / ${VAR} references in s against the given
// env map. matches os.Expand semantics. unset variables expand to
// "" (same as a shell with set -u off).
func expand(s string, env map[string]string) string {
	return os.Expand(s, func(k string) string { return env[k] })
}

func envSliceToMap(envv []string) map[string]string {
	m := make(map[string]string, len(envv))
	for _, kv := range envv {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	return m
}

func mapToEnv(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// hostHasOwnUserland returns true when /lib/<triplet> exists as a
// real directory rather than a symlink (or missing). the production
// scratch container has nothing under /lib until wireBaseFs symlinks
// the base snap's userland in; an ubuntu host has glibc + friends
// already. when true, snap run trusts the host's userland and skips
// the base-snap LD_LIBRARY_PATH / PATH overrides, which would
// otherwise pair the host's /bin/sh against the base snap's libc.
func hostHasOwnUserland() bool {
	st, err := os.Lstat(filepath.Join("/lib", multiarchTriplet()))
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeSymlink == 0 && st.IsDir()
}

// multiarchTriplet maps the dpkg arch to the multiarch lib subdir
// name. needed because base snaps ship libs under
// /usr/lib/<triplet>/, not the host arch name.
func multiarchTriplet() string {
	switch arch.DpkgArchitecture() {
	case "amd64":
		return "x86_64-linux-gnu"
	case "arm64":
		return "aarch64-linux-gnu"
	case "armhf":
		return "arm-linux-gnueabihf"
	case "i386":
		return "i386-linux-gnu"
	case "ppc64el":
		return "powerpc64le-linux-gnu"
	case "s390x":
		return "s390x-linux-gnu"
	case "riscv64":
		return "riscv64-linux-gnu"
	}
	return arch.DpkgArchitecture() + "-linux-gnu"
}
