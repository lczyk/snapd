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
	root := filepath.Join(snapMountDir, name)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return fmt.Errorf("not installed")
	}

	// stop daemons before deleting the snap tree; snap.yaml must still
	// be readable for declaredDaemons to work.
	stopDaemonsForSnap(name)

	// drop /snap/bin/<x> shims that point at the snap's apps. listing
	// /snap/bin and removing every link whose name starts with the
	// snap-name (either bare for the same-named app or "<name>." for
	// extras) covers the wireBins layout.
	if entries, err := os.ReadDir(snapBinDir); err == nil {
		for _, e := range entries {
			n := e.Name()
			if n == name || (len(n) > len(name)+1 && n[:len(name)] == name && n[len(name)] == '.') {
				_ = os.Remove(filepath.Join(snapBinDir, n))
			}
		}
	}

	// remove all extracted revisions, the current symlink, the dir.
	if err := os.RemoveAll(root); err != nil {
		return err
	}

	// per-snap data. /var/lib/snapd/snaps is conceptually a download
	// cache, not "installed state": removing /snap/<name> frees the
	// extracted tree, but the cache blob is content-addressed by sha
	// and harmless to keep. don't touch it -- under bind-mounted /var
	// /lib/snapd/snaps (parallel spread workers, shared persistent
	// cache, ...) deleting here would race with another process's
	// in-flight install.
	_ = os.RemoveAll(filepath.Join(snapDataDir, name))

	fmt.Printf("removed %s\n", name)
	return nil
}
