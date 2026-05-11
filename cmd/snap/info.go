// `snap info <name>` -- print a summary for the named snap. checks
// the local install first (so offline use works for installed snaps)
// and falls back to the store. shows the latest store revision
// alongside the installed one when both differ.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/store"
)

func cmdInfo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("info needs a snap name")
	}
	for i, name := range args {
		if i > 0 {
			fmt.Println("---")
		}
		if err := infoOne(name); err != nil {
			return fmt.Errorf("info %s: %w", name, err)
		}
	}
	return nil
}

func infoOne(name string) error {
	var local *snap.Info
	if isInstalled(name) {
		yamlBytes, err := os.ReadFile(filepath.Join(snapMountDir, name, "current", "meta", "snap.yaml"))
		if err == nil {
			info, err := snap.InfoFromSnapYaml(yamlBytes)
			if err == nil {
				local = info
				rev, _ := os.Readlink(filepath.Join(snapMountDir, name, "current"))
				if r, err := snap.ParseRevision(filepath.Base(rev)); err == nil {
					info.Revision = r
				}
			}
		}
	}

	st := store.New(nil, nil)
	remote, remoteErr := st.SnapInfo(context.Background(), store.SnapSpec{Name: name}, nil)

	switch {
	case local == nil && remote == nil:
		return fmt.Errorf("not found locally or in the store: %v", remoteErr)
	case local == nil:
		printSnap("(store)", remote)
	case remote == nil:
		printSnap("(installed; store unreachable)", local)
	default:
		printSnap("installed", local)
		fmt.Println()
		if remote.Revision != local.Revision {
			fmt.Printf("store has revision %s (you have %s)\n", remote.Revision, local.Revision)
		} else {
			fmt.Println("up to date with the store")
		}
	}
	return nil
}

func printSnap(label string, info *snap.Info) {
	fmt.Printf("name:       %s\n", info.SnapName())
	fmt.Printf("status:     %s\n", label)
	fmt.Printf("version:    %s\n", info.Version)
	fmt.Printf("revision:   %s\n", info.Revision)
	fmt.Printf("type:       %s\n", info.Type())
	if info.Base != "" {
		fmt.Printf("base:       %s\n", info.Base)
	}
	if info.Confinement != "" {
		fmt.Printf("confinement:%s\n", info.Confinement)
	}
	if info.Summary() != "" {
		fmt.Printf("summary:    %s\n", info.Summary())
	}
	if len(info.Apps) > 0 {
		var apps []string
		for n := range info.Apps {
			apps = append(apps, n)
		}
		fmt.Printf("apps:       %v\n", apps)
	}
}
