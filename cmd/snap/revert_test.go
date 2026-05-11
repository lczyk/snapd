package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/snapcore/snapd/snap"
)

// setupFakeSnap creates a minimal /snap/<name>/<rev>/meta/snap.yaml
// tree under root and returns the mount dir for that revision.
func setupFakeSnap(t *testing.T, root, name string, revs []string, current string) {
	t.Helper()
	for _, rev := range revs {
		d := filepath.Join(root, "snap", name, rev, "meta")
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		yaml := "name: " + name + "\nversion: 1.0\n"
		if err := os.WriteFile(filepath.Join(d, "snap.yaml"), []byte(yaml), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cur := filepath.Join(root, "snap", name, "current")
	_ = os.Remove(cur)
	if err := os.Symlink(current, cur); err != nil {
		t.Fatal(err)
	}
}

// withSnapRoot overrides the package-level path vars for the duration of
// the test, restoring them on cleanup.
func withSnapRoot(t *testing.T, root string) {
	t.Helper()
	oldMount := snapMountDir
	oldBin := snapBinDir
	oldData := snapDataDir
	oldDl := snapDownloadDir
	oldAsserts := snapAssertsDir
	oldSup := supervisorDir
	oldLog := daemonLogDir

	snapMountDir = filepath.Join(root, "snap")
	snapBinDir = filepath.Join(root, "snap", "bin")
	snapDataDir = filepath.Join(root, "var", "snap")
	snapDownloadDir = filepath.Join(root, "var", "lib", "snapd", "snaps")
	snapAssertsDir = filepath.Join(root, "var", "lib", "snapd", "assertions")
	supervisorDir = filepath.Join(root, "run", "snapd", "supervisors")
	daemonLogDir = filepath.Join(root, "var", "log", "snapd")

	t.Cleanup(func() {
		snapMountDir = oldMount
		snapBinDir = oldBin
		snapDataDir = oldData
		snapDownloadDir = oldDl
		snapAssertsDir = oldAsserts
		supervisorDir = oldSup
		daemonLogDir = oldLog
	})
}

func TestInstalledRevisions(t *testing.T) {
	root := t.TempDir()
	withSnapRoot(t, root)

	setupFakeSnap(t, root, "foo", []string{"28", "29"}, "29")

	revs, err := installedRevisions("foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(revs) != 2 {
		t.Fatalf("want 2 revisions, got %d: %v", len(revs), revs)
	}
	if revs[0] != snap.R(28) || revs[1] != snap.R(29) {
		t.Errorf("want [28 29], got %v", revs)
	}
}

func TestCurrentRevision(t *testing.T) {
	root := t.TempDir()
	withSnapRoot(t, root)

	setupFakeSnap(t, root, "foo", []string{"28", "29"}, "29")

	cur, err := currentRevision("foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cur != snap.R(29) {
		t.Errorf("want rev 29, got %v", cur)
	}
}

func TestPreviousRevision(t *testing.T) {
	t.Run("picks non-current", func(t *testing.T) {
		root := t.TempDir()
		withSnapRoot(t, root)
		setupFakeSnap(t, root, "foo", []string{"28", "29"}, "29")

		prev, err := previousRevision("foo", snap.R(29))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if prev != snap.R(28) {
			t.Errorf("want 28, got %v", prev)
		}
	})

	t.Run("no previous revision errors", func(t *testing.T) {
		root := t.TempDir()
		withSnapRoot(t, root)
		setupFakeSnap(t, root, "foo", []string{"29"}, "29")

		_, err := previousRevision("foo", snap.R(29))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("three revisions picks highest non-current", func(t *testing.T) {
		root := t.TempDir()
		withSnapRoot(t, root)
		setupFakeSnap(t, root, "foo", []string{"27", "28", "29"}, "29")

		prev, err := previousRevision("foo", snap.R(29))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if prev != snap.R(28) {
			t.Errorf("want 28, got %v", prev)
		}
	})
}

func TestCmdRevert(t *testing.T) {
	t.Run("reverts to previous revision", func(t *testing.T) {
		root := t.TempDir()
		withSnapRoot(t, root)
		setupFakeSnap(t, root, "foo", []string{"28", "29"}, "29")

		if err := cmdRevert([]string{"foo"}); err != nil {
			t.Fatalf("cmdRevert: %v", err)
		}

		cur, err := currentRevision("foo")
		if err != nil {
			t.Fatalf("read current after revert: %v", err)
		}
		if cur != snap.R(28) {
			t.Errorf("want current=28 after revert, got %v", cur)
		}
	})

	t.Run("--revision flag honoured", func(t *testing.T) {
		root := t.TempDir()
		withSnapRoot(t, root)
		setupFakeSnap(t, root, "foo", []string{"27", "28", "29"}, "29")

		if err := cmdRevert([]string{"--revision=27", "foo"}); err != nil {
			t.Fatalf("cmdRevert --revision=27: %v", err)
		}

		cur, err := currentRevision("foo")
		if err != nil {
			t.Fatalf("read current after revert: %v", err)
		}
		if cur != snap.R(27) {
			t.Errorf("want current=27, got %v", cur)
		}
	})

	t.Run("already at target revision errors", func(t *testing.T) {
		root := t.TempDir()
		withSnapRoot(t, root)
		setupFakeSnap(t, root, "foo", []string{"29"}, "29")

		err := cmdRevert([]string{"--revision=29", "foo"})
		if err == nil {
			t.Fatal("expected error when already at target revision")
		}
	})

	t.Run("no snap name errors", func(t *testing.T) {
		if err := cmdRevert(nil); err == nil {
			t.Fatal("expected error for missing snap name")
		}
	})

	t.Run("not installed errors", func(t *testing.T) {
		root := t.TempDir()
		withSnapRoot(t, root)

		err := cmdRevert([]string{"notinstalled"})
		if err == nil {
			t.Fatal("expected error for uninstalled snap")
		}
	})
}

func TestPruneOldRevisionsKeepsPrevious(t *testing.T) {
	root := t.TempDir()
	withSnapRoot(t, root)

	// simulate: revs 27, 28, 29 on disk; 29 is current
	setupFakeSnap(t, root, "foo", []string{"27", "28", "29"}, "29")

	info := &snap.Info{}
	info.SideInfo = snap.SideInfo{RealName: "foo", Revision: snap.R(29)}

	if err := pruneOldRevisions(info); err != nil {
		t.Fatalf("pruneOldRevisions: %v", err)
	}

	revs, err := installedRevisions("foo")
	if err != nil {
		t.Fatalf("installedRevisions: %v", err)
	}
	// should keep 28 (previous) and 29 (current); prune 27
	if len(revs) != 2 {
		t.Fatalf("want 2 revisions after prune, got %d: %v", len(revs), revs)
	}
	if revs[0] != snap.R(28) || revs[1] != snap.R(29) {
		t.Errorf("want [28 29] after prune, got %v", revs)
	}
}
