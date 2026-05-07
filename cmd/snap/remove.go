// `snap remove <name>` -- delete /snap/<name>/, /var/snap/<name>/
// and the cached download. drop the /snap/bin/<app> shims for any
// app the snap declared.
//
// no purge / snapshot logic; the prototype's lifecycle is just
// "extracted on disk" / "not extracted on disk".

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func cmdRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("remove needs at least one snap name")
	}
	for _, name := range args {
		if err := removeOne(name); err != nil {
			return fmt.Errorf("remove %s: %w", name, err)
		}
	}
	return nil
}

func removeOne(name string) error {
	root := filepath.Join("/snap", name)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return fmt.Errorf("not installed")
	}

	// drop /snap/bin/<x> shims that point at the snap's apps. listing
	// /snap/bin and removing every link whose name starts with the
	// snap-name (either bare for the same-named app or "<name>." for
	// extras) covers the wireBins layout.
	if entries, err := os.ReadDir("/snap/bin"); err == nil {
		for _, e := range entries {
			n := e.Name()
			if n == name || (len(n) > len(name)+1 && n[:len(name)] == name && n[len(name)] == '.') {
				_ = os.Remove(filepath.Join("/snap/bin", n))
			}
		}
	}

	// remove all extracted revisions, the current symlink, the dir.
	if err := os.RemoveAll(root); err != nil {
		return err
	}

	// per-snap data dirs and download cache. ignore missing.
	_ = os.RemoveAll(filepath.Join("/var/snap", name))
	if dirEntries, err := os.ReadDir("/var/lib/snapd/snaps"); err == nil {
		for _, e := range dirEntries {
			if matchesPrefix(e.Name(), name+"_") {
				_ = os.Remove(filepath.Join("/var/lib/snapd/snaps", e.Name()))
			}
		}
	}

	fmt.Printf("removed %s\n", name)
	return nil
}

func matchesPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}
