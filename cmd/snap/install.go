// `snap install <name>` -- fetch from the store and unsquash to
// /snap/<name>/<rev>/. recursively installs the declared base
// (core24, core22, ...) when missing. wires up /snap/bin/<app>
// shims pointing back at this binary so `snap run <app>` works.
//
// no daemon, no state file. the on-disk layout is the only state:
// the presence of /snap/<name>/<rev>/meta/snap.yaml means it's
// installed. /snap/<name>/current symlinks the latest revision.

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/snapcore/snapd/progress"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/snap/squashfs"
	"github.com/snapcore/snapd/store"
)

func cmdInstall(args []string) error {
	channel, args := extractChannel(args)
	if len(args) == 0 {
		return fmt.Errorf("install needs a snap name")
	}
	for _, name := range args {
		if err := installOne(name, channel); err != nil {
			return fmt.Errorf("install %s: %w", name, err)
		}
	}
	return nil
}

// extractChannel pulls --channel=<x> / --channel <x> out of args and
// returns the channel + the remaining positional args. unknown flags
// are passed through (cli surface is small enough that we don't need
// a full flag library).
func extractChannel(args []string) (string, []string) {
	var ch string
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "--channel="):
			ch = strings.TrimPrefix(a, "--channel=")
		case a == "--channel" && i+1 < len(args):
			ch = args[i+1]
			i++
		default:
			out = append(out, a)
		}
	}
	return ch, out
}

// channelPath holds the channel a snap was last installed / refreshed
// from, so a subsequent `snap refresh` w/out an explicit --channel
// stays on the same track.
func channelPath(name string) string {
	return filepath.Join("/snap", name, ".channel")
}

func saveChannel(name, channel string) error {
	if channel == "" {
		return nil
	}
	return os.WriteFile(channelPath(name), []byte(channel+"\n"), 0644)
}

func readChannel(name string) string {
	b, err := os.ReadFile(channelPath(name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// installOne does the fetch + unsquash + base-recursion + bin-wire dance
// for a single snap. idempotent on the snap name + revision: if the
// revision is already extracted under /snap/<name>/<rev>/, it skips
// the download but still re-runs the post-install steps.
//
// when name ends in .snap and is a regular file, treat it as a
// sideload: skip the store and assertion verification, parse the
// snap.yaml from the local file, extract under a synthetic xN
// revision so it doesn't collide with store-fetched revs.
func installOne(name, channel string) error {
	if strings.HasSuffix(name, ".snap") {
		if _, err := os.Stat(name); err == nil {
			return installLocal(name)
		}
	}
	st := store.New(nil, nil)
	ctx := context.Background()

	info, err := storeInfoForChannel(ctx, st, name, channel)
	if err != nil {
		return fmt.Errorf("fetch info: %w", err)
	}

	mountDir := filepath.Join("/snap", info.SnapName(), info.Revision.String())
	if _, statErr := os.Stat(filepath.Join(mountDir, "meta", "snap.yaml")); statErr == nil {
		fmt.Printf("%s %s already installed at %s\n", info.SnapName(), info.Revision, mountDir)
	} else {
		if err := download(ctx, st, info); err != nil {
			return err
		}
		fmt.Printf("verifying assertions for %s\n", info.SnapName())
		if err := verifyAssertions(st, info, snapDownloadPath(info)); err != nil {
			// don't leave the unverified .snap on disk -- a future
			// install attempt would skip the download (sha cache hit
			// in store.Download) and pick up the bad blob.
			_ = os.Remove(snapDownloadPath(info))
			return fmt.Errorf("verify: %w", err)
		}
		if err := extractTo(snapDownloadPath(info), mountDir); err != nil {
			return err
		}
		fmt.Printf("installed %s %s -> %s\n", info.SnapName(), info.Revision, mountDir)
	}

	if err := updateCurrent(info); err != nil {
		return fmt.Errorf("update current symlink: %w", err)
	}

	// after pointing current at the new revision, drop everything else
	// under /snap/<name>/ that isn't the current rev. without this,
	// every install/refresh cycle leaves the previous revision sitting
	// on disk forever.
	if err := pruneOldRevisions(info); err != nil {
		return fmt.Errorf("prune old revisions: %w", err)
	}

	// re-read snap.yaml from the extracted tree -- it has the apps
	// and the declared base (which info.Base from the store should
	// also report, but reading from the extracted file is the source
	// of truth once extracted).
	yamlBytes, err := os.ReadFile(filepath.Join(mountDir, "meta", "snap.yaml"))
	if err != nil {
		return fmt.Errorf("read snap.yaml: %w", err)
	}
	installedInfo, err := snap.InfoFromSnapYaml(yamlBytes)
	if err != nil {
		return fmt.Errorf("parse snap.yaml: %w", err)
	}

	// snaps without an explicit `base:` historically default to the
	// legacy `core` os snap (which provides /bin/sh, libc, ...).
	// pick something modern instead -- `core` still works but is
	// frozen on 16.04; core22 is supported and small enough.
	base := installedInfo.Base
	if base == "" {
		// don't recurse on a base-less base (e.g. `core` itself sets
		// no base in its yaml because it *is* the base).
		if t := installedInfo.Type(); t != snap.TypeOS && t != snap.TypeBase {
			base = "core22"
		}
	}
	if base != "" && base != "none" && base != "bare" {
		if !isInstalled(base) {
			fmt.Printf("installing base %s\n", base)
			if err := installOne(base, ""); err != nil {
				return fmt.Errorf("install base %s: %w", base, err)
			}
		}
	}

	if err := saveChannel(info.SnapName(), channel); err != nil {
		return fmt.Errorf("save channel: %w", err)
	}

	if err := wireBins(installedInfo, mountDir); err != nil {
		return fmt.Errorf("wire /snap/bin shims: %w", err)
	}
	// base / os snaps own the userland everything else needs (libc,
	// ld-linux-*, /bin/sh, ...). expose a few of those at standard
	// host paths so dynamic snap binaries can resolve their ELF
	// interpreter and shebangs work.
	if t := installedInfo.Type(); t == snap.TypeOS || t == snap.TypeBase {
		if err := wireBaseFs(installedInfo, mountDir); err != nil {
			return fmt.Errorf("wire base fs: %w", err)
		}
	}
	return nil
}

// storeInfoForChannel resolves a snap.Info from the store, picking the
// requested channel when one is given. the v2 info endpoint always
// returns latest/stable as ChannelMap[0]; for any other channel we
// have to go through SnapAction with action=install, which the store
// resolves against the requested channel and hands back a snap.Info
// pointing at the right revision + download url.
func storeInfoForChannel(ctx context.Context, st *store.Store, name, channel string) (*snap.Info, error) {
	if channel == "" {
		return st.SnapInfo(ctx, store.SnapSpec{Name: name}, nil)
	}
	sars, _, err := st.SnapAction(ctx, nil, []*store.SnapAction{{
		Action:       "install",
		InstanceName: name,
		Channel:      channel,
	}}, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if len(sars) == 0 || sars[0].Info == nil {
		return nil, fmt.Errorf("no result for %s on channel %s", name, channel)
	}
	return sars[0].Info, nil
}

func download(ctx context.Context, st *store.Store, info *snap.Info) error {
	target := snapDownloadPath(info)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	pbar := progress.MakeProgressBar(os.Stdout)
	defer pbar.Finished()
	if err := st.Download(ctx, info.SnapName(), target, &info.DownloadInfo, pbar, nil, nil); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	return nil
}

func snapDownloadPath(info *snap.Info) string {
	return filepath.Join("/var/lib/snapd/snaps",
		fmt.Sprintf("%s_%s.snap", info.SnapName(), info.Revision))
}

func extractTo(snapPath, mountDir string) error {
	if err := os.MkdirAll(mountDir, 0755); err != nil {
		return err
	}
	sq := squashfs.New(snapPath)
	if err := sq.Unpack("*", mountDir); err != nil {
		return fmt.Errorf("unsquash: %w", err)
	}
	return nil
}

// updateCurrent points /snap/<name>/current at the just-installed rev.
// uses an atomic rename of a temp symlink so concurrent installs of
// the same snap don't see a half-broken pointer.
func updateCurrent(info *snap.Info) error {
	parent := filepath.Join("/snap", info.SnapName())
	cur := filepath.Join(parent, "current")
	tmp := filepath.Join(parent, ".current.new")
	_ = os.Remove(tmp)
	if err := os.Symlink(info.Revision.String(), tmp); err != nil {
		return err
	}
	return os.Rename(tmp, cur)
}

func isInstalled(name string) bool {
	_, err := os.Stat(filepath.Join("/snap", name, "current", "meta", "snap.yaml"))
	return err == nil
}

// installLocal handles `snap install ./foo.snap`. no store, no
// assertion chain -- the user is asserting they trust the file.
// the revision is synthesised as xN where N is the next free
// number under /snap/<name>/, matching how upstream tags
// sideloaded revisions.
func installLocal(snapPath string) error {
	abs, err := filepath.Abs(snapPath)
	if err != nil {
		return err
	}

	// peek at the snap.yaml inside the .snap to learn the snap's name
	// before we know where to put it.
	sq := squashfs.New(abs)
	yamlBytes, err := sq.ReadFile("meta/snap.yaml")
	if err != nil {
		return fmt.Errorf("read snap.yaml from %s: %w", abs, err)
	}
	info, err := snap.InfoFromSnapYaml(yamlBytes)
	if err != nil {
		return fmt.Errorf("parse snap.yaml: %w", err)
	}

	rev, err := nextLocalRevision(info.SnapName())
	if err != nil {
		return err
	}
	info.Revision = rev
	mountDir := filepath.Join("/snap", info.SnapName(), rev.String())

	if err := extractTo(abs, mountDir); err != nil {
		return err
	}
	fmt.Printf("installed %s %s -> %s (sideloaded, unverified)\n",
		info.SnapName(), info.Revision, mountDir)

	if err := updateCurrent(info); err != nil {
		return fmt.Errorf("update current symlink: %w", err)
	}
	if err := pruneOldRevisions(info); err != nil {
		return fmt.Errorf("prune old revisions: %w", err)
	}

	if base := info.Base; base != "" && base != "none" && base != "bare" {
		if !isInstalled(base) {
			fmt.Printf("installing base %s\n", base)
			if err := installOne(base, ""); err != nil {
				return fmt.Errorf("install base %s: %w", base, err)
			}
		}
	}

	if err := wireBins(info, mountDir); err != nil {
		return fmt.Errorf("wire /snap/bin shims: %w", err)
	}
	if t := info.Type(); t == snap.TypeOS || t == snap.TypeBase {
		if err := wireBaseFs(info, mountDir); err != nil {
			return fmt.Errorf("wire base fs: %w", err)
		}
	}
	return nil
}

// nextLocalRevision returns x1, x2, ... -- the next available
// sideload revision tag for /snap/<name>/.
func nextLocalRevision(name string) (snap.Revision, error) {
	entries, err := os.ReadDir(filepath.Join("/snap", name))
	if err != nil && !os.IsNotExist(err) {
		return snap.Revision{}, err
	}
	max := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		r, err := snap.ParseRevision(e.Name())
		if err != nil {
			continue
		}
		if r.Local() && -r.N > max {
			max = -r.N
		}
	}
	return snap.R(-(max + 1)), nil
}

// pruneOldRevisions removes /snap/<name>/<rev> dirs that aren't the
// current revision. called after a fresh install / refresh so we
// don't leak storage on each update cycle.
func pruneOldRevisions(info *snap.Info) error {
	parent := filepath.Join("/snap", info.SnapName())
	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	keep := info.Revision.String()
	for _, e := range entries {
		n := e.Name()
		if n == "current" || n == ".current.new" || n == keep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(parent, n)); err != nil {
			return err
		}
		// also drop the cached download for the old rev.
		_ = os.Remove(filepath.Join("/var/lib/snapd/snaps",
			fmt.Sprintf("%s_%s.snap", info.SnapName(), n)))
	}
	return nil
}

// wireBins makes /snap/bin/<app> -> /usr/bin/snap so that running the
// shim ends up in cmdRun, which sets up env and execs the real app.
// this binary itself must live at /usr/bin/snap; if the user installed
// it elsewhere, set SNAP_SELF to point at it.
func wireBins(info *snap.Info, _ string) error {
	if err := os.MkdirAll("/snap/bin", 0755); err != nil {
		return err
	}
	target := os.Getenv("SNAP_SELF")
	if target == "" {
		target = "/usr/bin/snap"
	}
	for appName := range info.Apps {
		var link string
		if appName == info.SnapName() {
			link = filepath.Join("/snap/bin", appName)
		} else {
			link = filepath.Join("/snap/bin", info.SnapName()+"."+appName)
		}
		// symlink races are easier to ignore than fight: rm first.
		_ = os.Remove(link)
		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("symlink %s: %w", link, err)
		}
	}
	return nil
}

// wireBaseFs symlinks the base snap's dynamic linker, /bin/sh, and
// multiarch lib dir into host-level paths so non-snap-confined
// binaries find them. only relevant for base / os snaps.
//
// without this:
//   - dynamic snap binaries fail with "no such file or directory" on
//     exec because their hardcoded ELF interpreter (/lib/ld-linux-*
//     or /lib64/ld-linux-*) doesn't exist
//   - shebang scripts (#!/bin/sh ...) fail for the same reason
func wireBaseFs(info *snap.Info, _ string) error {
	baseRoot := filepath.Join("/snap", info.SnapName(), "current")

	// idempotent symlink: rm + ln -s
	link := func(target, linkPath string) error {
		if _, err := os.Stat(target); err != nil {
			// target not present in this base -- skip silently;
			// e.g. /usr/lib/x86_64-linux-gnu doesn't exist in
			// an arm64-only base.
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
			return err
		}
		_ = os.Remove(linkPath)
		return os.Symlink(target, linkPath)
	}

	// dynamic linker. base snaps put it at /usr/lib/<triplet>/ld-linux-*.so.*
	// or at /usr/lib/ld-linux-*.so.*. expose at /lib and /lib64 since
	// different binaries hardcode different paths.
	for _, t := range []string{"aarch64-linux-gnu", "x86_64-linux-gnu", "arm-linux-gnueabihf"} {
		archDir := filepath.Join(baseRoot, "usr/lib", t)
		entries, err := os.ReadDir(archDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "ld-linux") || !strings.Contains(name, ".so") {
				continue
			}
			if err := link(filepath.Join(archDir, name), filepath.Join("/lib", name)); err != nil {
				return err
			}
			if err := link(filepath.Join(archDir, name), filepath.Join("/lib64", name)); err != nil {
				return err
			}
		}
		// also symlink the whole multiarch dir at /lib/<triplet> so
		// DT_NEEDED entries (libc.so.6, libpthread.so.0, ...) resolve
		// without LD_LIBRARY_PATH gymnastics.
		if err := link(archDir, filepath.Join("/lib", t)); err != nil {
			return err
		}
	}

	// /bin/sh -> base's bash. covers #!/bin/sh shebangs.
	for _, sh := range []string{
		filepath.Join(baseRoot, "usr/bin/bash"),
		filepath.Join(baseRoot, "bin/bash"),
		filepath.Join(baseRoot, "usr/bin/sh"),
		filepath.Join(baseRoot, "bin/sh"),
	} {
		if _, err := os.Stat(sh); err == nil {
			if err := link(sh, "/bin/sh"); err != nil {
				return err
			}
			break
		}
	}

	// /usr/bin/* and /bin/* from the base, for shebangs like
	// "#!/usr/bin/env bash" and snap-shipped scripts that call
	// out to standard utilities (sed, grep, ls, ...). preserve
	// /usr/bin/snap (this binary) so the shim chain keeps working.
	for _, src := range []struct{ from, to string }{
		{filepath.Join(baseRoot, "usr/bin"), "/usr/bin"},
		{filepath.Join(baseRoot, "bin"), "/bin"},
		{filepath.Join(baseRoot, "usr/sbin"), "/usr/sbin"},
		{filepath.Join(baseRoot, "sbin"), "/sbin"},
	} {
		entries, err := os.ReadDir(src.from)
		if err != nil {
			continue
		}
		if err := os.MkdirAll(src.to, 0755); err != nil {
			return err
		}
		for _, e := range entries {
			dst := filepath.Join(src.to, e.Name())
			if dst == "/usr/bin/snap" || dst == "/bin/snap" {
				// don't clobber the snap multitool itself.
				continue
			}
			// only link if nothing's there or what's there is a
			// stale symlink to a previous base; an existing
			// non-symlink (the snap binary copied to /bin from
			// the docker COPY) is left alone.
			if existing, err := os.Lstat(dst); err == nil {
				if existing.Mode()&os.ModeSymlink == 0 {
					continue
				}
				_ = os.Remove(dst)
			}
			if err := os.Symlink(filepath.Join(src.from, e.Name()), dst); err != nil {
				return err
			}
		}
	}
	return nil
}

// io.Discard placeholder so the import isn't dropped if I add a
// Reader path later.
var _ = io.Discard
