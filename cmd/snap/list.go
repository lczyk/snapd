// `snap list` -- show what's installed under /snap/<name>/<rev>/.
// the on-disk layout is the only state, so listing == ls /snap/.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/snapcore/snapd/snap"
)

func cmdList(_ []string) error {
	entries, err := os.ReadDir(snapMountDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	type row struct {
		name, version, revision, channel, snapType string
	}
	var rows []row
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "bin" {
			continue
		}
		yamlPath := filepath.Join(snapMountDir, e.Name(), "current", "meta", "snap.yaml")
		yamlBytes, err := os.ReadFile(yamlPath)
		if err != nil {
			continue
		}
		info, err := snap.InfoFromSnapYaml(yamlBytes)
		if err != nil {
			continue
		}
		// resolve revision via current symlink
		rev, err := os.Readlink(filepath.Join(snapMountDir, e.Name(), "current"))
		if err != nil {
			continue
		}
		ch := readChannel(e.Name())
		if ch == "" {
			ch = "-"
		}
		rows = append(rows, row{
			name:     e.Name(),
			version:  info.Version,
			revision: filepath.Base(rev),
			channel:  ch,
			snapType: string(info.Type()),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Name\tVersion\tRev\tTracking\tType")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.name, r.version, r.revision, r.channel, r.snapType)
	}
	return w.Flush()
}
