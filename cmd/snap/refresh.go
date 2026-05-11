// `snap refresh [<name>...]` -- re-run install for each named snap;
// with no args, refresh everything currently under /snap/.
//
// the install path is already idempotent (skip if /snap/<name>/<rev>/
// is already there for the latest rev) and prunes old revisions on
// successful update, so refresh is just install over the same names.

package main

import (
	"fmt"
	"os"
)

func cmdRefresh(args []string) error {
	channel, args := extractFlags(args)
	if len(args) == 0 {
		// refresh everything under /snap/, skipping bin
		entries, err := os.ReadDir(snapMountDir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		for _, e := range entries {
			if !e.IsDir() || e.Name() == "bin" {
				continue
			}
			if !isInstalled(e.Name()) {
				continue
			}
			args = append(args, e.Name())
		}
		if len(args) == 0 {
			fmt.Println("nothing to refresh")
			return nil
		}
	}
	for _, name := range args {
		if !isInstalled(name) {
			return fmt.Errorf("refresh %s: not installed", name)
		}
		// without an explicit --channel, stay on whatever channel the
		// snap was last installed / refreshed from. falls through to
		// latest/stable when nothing was recorded.
		ch := channel
		if ch == "" {
			ch = readChannel(name)
		}
		if err := installOne(name, ch); err != nil {
			return fmt.Errorf("refresh %s: %w", name, err)
		}
	}
	return nil
}
