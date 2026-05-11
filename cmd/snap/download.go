// `snap download <name>` -- fetch the snap and its assertion bundle to
// the current directory (or --target-directory). mirrors the output of
// the real `snap download` command:
//
//	Fetching snap "hello"
//	Fetching assertions for "hello"
//	Install the snap with:
//	   snap install hello_29.snap

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/asserts/sysdb"
	"github.com/snapcore/snapd/progress"
	"github.com/snapcore/snapd/store"
)

func cmdDownload(args []string) error {
	var channel, targetDir, basename string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "--channel="):
			channel = strings.TrimPrefix(a, "--channel=")
		case a == "--channel" && i+1 < len(args):
			channel = args[i+1]
			i++
		case strings.HasPrefix(a, "--target-directory="):
			targetDir = strings.TrimPrefix(a, "--target-directory=")
		case a == "--target-directory" && i+1 < len(args):
			targetDir = args[i+1]
			i++
		case strings.HasPrefix(a, "--basename="):
			basename = strings.TrimPrefix(a, "--basename=")
		case a == "--basename" && i+1 < len(args):
			basename = args[i+1]
			i++
		case a == "--edge":
			channel = "latest/edge"
		case a == "--beta":
			channel = "latest/beta"
		case a == "--candidate":
			channel = "latest/candidate"
		case a == "--stable":
			channel = "latest/stable"
		case strings.HasPrefix(a, "--revision="):
			// revision-pinned downloads are not yet implemented; warn
			// and fall through to the channel-based path.
			fmt.Fprintf(os.Stderr, "warning: --revision is not yet supported; downloading latest\n")
		case a == "--revision" && i+1 < len(args):
			fmt.Fprintf(os.Stderr, "warning: --revision is not yet supported; downloading latest\n")
			i++ // consume the value
		default:
			rest = append(rest, a)
		}
	}

	if len(rest) == 0 {
		return fmt.Errorf("download needs a snap name")
	}
	name := rest[0]

	if targetDir == "" {
		targetDir = "."
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("mkdir target: %w", err)
	}

	ctx := context.Background()
	st := store.New(nil, nil)

	info, err := storeInfoForChannel(ctx, st, name, channel)
	if err != nil {
		return fmt.Errorf("fetch info: %w", err)
	}

	base := basename
	if base == "" {
		base = fmt.Sprintf("%s_%s", info.SnapName(), info.Revision)
	}
	snapDest := filepath.Join(targetDir, base+".snap")
	assertDest := filepath.Join(targetDir, base+".assert")

	fmt.Printf("Fetching snap %q\n", info.SnapName())
	pbar := progress.MakeProgressBar(os.Stdout)
	if err := st.Download(ctx, info.SnapName(), snapDest, &info.DownloadInfo, pbar, nil, nil); err != nil {
		pbar.Finished()
		return fmt.Errorf("download: %w", err)
	}
	pbar.Finished()

	fmt.Printf("Fetching assertions for %q\n", info.SnapName())
	if err := writeAssertions(st, snapDest, assertDest); err != nil {
		return fmt.Errorf("fetch assertions: %w", err)
	}

	snapFile := filepath.Base(snapDest)
	fmt.Printf("Install the snap with:\n   snap install %s\n", snapFile)
	return nil
}

// writeAssertions fetches the snap-revision assertion chain for the
// downloaded snap and writes all assertions to assertDest as a
// newline-separated assertion stream (same format as real snap download).
func writeAssertions(s *store.Store, snapPath, assertDest string) error {
	hash, _, err := asserts.SnapFileSHA3_384(snapPath)
	if err != nil {
		return fmt.Errorf("hash snap: %w", err)
	}

	if err := os.MkdirAll(snapAssertsDir, 0755); err != nil {
		return fmt.Errorf("mkdir asserts db: %w", err)
	}
	persistDB, err := sysdb.OpenAt(snapAssertsDir)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	f, err := os.Create(assertDest)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := asserts.NewEncoder(f)

	db := persistDB.WithStackedBackstore(asserts.NewMemoryBackstore())
	retrieve := func(ref *asserts.Ref) (asserts.Assertion, error) {
		return s.Assertion(ref.Type, ref.PrimaryKey, nil)
	}
	save := func(a asserts.Assertion) error {
		// write to the file stream
		if err := enc.Encode(a); err != nil {
			return err
		}
		// also add to db (needed for prereq resolution)
		addErr := db.Add(a)
		if _, dup := addErr.(*asserts.RevisionError); dup {
			return nil
		}
		return addErr
	}
	fetcher := asserts.NewFetcher(db, retrieve, save)

	revRef := &asserts.Ref{
		Type:       asserts.SnapRevisionType,
		PrimaryKey: []string{hash},
	}
	return fetcher.Fetch(revRef)
}
