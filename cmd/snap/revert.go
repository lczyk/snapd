// `snap revert <name>` -- switch the current symlink back to the
// previous on-disk revision, matching the real snap CLI's output:
//
//	hello reverted to 28
//
// requires that there be a previous revision on disk; `pruneOldRevisions`
// retains the last two revisions so revert is always available after a
// refresh.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/snapcore/snapd/snap"
)

func cmdRevert(args []string) error {
	var targetRevStr string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "--revision="):
			targetRevStr = strings.TrimPrefix(a, "--revision=")
		case a == "--revision" && i+1 < len(args):
			targetRevStr = args[i+1]
			i++
		case a == "--devmode" || a == "--jailmode" || a == "--classic" || a == "--no-wait":
			// accepted for compat with the real CLI; noop in this build
		default:
			rest = append(rest, a)
		}
	}

	if len(rest) == 0 {
		return fmt.Errorf("revert needs a snap name")
	}
	name := rest[0]

	if !isInstalled(name) {
		return fmt.Errorf("%s is not installed", name)
	}

	cur, err := currentRevision(name)
	if err != nil {
		return err
	}

	var target snap.Revision
	if targetRevStr != "" {
		target, err = snap.ParseRevision(targetRevStr)
		if err != nil {
			return fmt.Errorf("invalid revision %q: %w", targetRevStr, err)
		}
		if target == cur {
			return fmt.Errorf("already at revision %s", cur)
		}
		// verify the requested revision is actually on disk
		if _, statErr := os.Stat(filepath.Join(snapMountDir, name, target.String(), "meta", "snap.yaml")); statErr != nil {
			return fmt.Errorf("revision %s not available on disk (did it get pruned?)", target)
		}
	} else {
		target, err = previousRevision(name, cur)
		if err != nil {
			return err
		}
	}

	// stop daemons running from current rev before switching
	stopDaemonsForSnap(name)

	// atomically update the current symlink
	parent := filepath.Join(snapMountDir, name)
	tmp := filepath.Join(parent, ".current.new")
	_ = os.Remove(tmp)
	if err := os.Symlink(target.String(), tmp); err != nil {
		return fmt.Errorf("symlink: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(parent, "current")); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	// re-read snap.yaml from the reverted tree and re-wire bins + daemons
	mountDir := filepath.Join(snapMountDir, name, target.String())
	yamlBytes, err := os.ReadFile(filepath.Join(mountDir, "meta", "snap.yaml"))
	if err != nil {
		return fmt.Errorf("read snap.yaml: %w", err)
	}
	info, err := snap.InfoFromSnapYaml(yamlBytes)
	if err != nil {
		return fmt.Errorf("parse snap.yaml: %w", err)
	}

	if err := wireBins(info); err != nil {
		return fmt.Errorf("wire bins: %w", err)
	}
	if err := startDaemonsForSnap(info); err != nil {
		return fmt.Errorf("start daemons: %w", err)
	}

	fmt.Printf("%s reverted to %s\n", name, target)
	return nil
}

// currentRevision returns the revision the current symlink points at.
func currentRevision(name string) (snap.Revision, error) {
	link, err := os.Readlink(filepath.Join(snapMountDir, name, "current"))
	if err != nil {
		return snap.Revision{}, fmt.Errorf("read current symlink: %w", err)
	}
	return snap.ParseRevision(filepath.Base(link))
}

// previousRevision returns the most recent revision on disk that isn't cur.
// returns an error when there are no other revisions (i.e. nothing to revert to).
func previousRevision(name string, cur snap.Revision) (snap.Revision, error) {
	others, err := installedRevisions(name)
	if err != nil {
		return snap.Revision{}, err
	}
	// drop current from the list
	filtered := others[:0]
	for _, r := range others {
		if r != cur {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		return snap.Revision{}, fmt.Errorf("no previous revision available for %s (snap refresh first, then revert)", name)
	}
	// pick the highest-numbered one -- for store revisions that's the
	// most recent, for local (xN) revisions it's also the most recent.
	return filtered[len(filtered)-1], nil
}

// installedRevisions lists all revisions present on disk for name,
// sorted ascending by revision number.
func installedRevisions(name string) ([]snap.Revision, error) {
	entries, err := os.ReadDir(filepath.Join(snapMountDir, name))
	if err != nil {
		return nil, err
	}
	var revs []snap.Revision
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		r, err := snap.ParseRevision(e.Name())
		if err != nil {
			continue // "current", ".current.new", ".channel", etc.
		}
		revs = append(revs, r)
	}
	sort.Slice(revs, func(i, j int) bool { return revs[i].N < revs[j].N })
	return revs, nil
}
