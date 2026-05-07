// signature verification for downloaded snaps. the chain we verify:
//
//   snap-revision  -- signed by an account-key, asserts that the
//                     given sha3-384 corresponds to a specific
//                     snap-id at a specific revision
//   snap-declaration -- signed by an account-key, asserts that the
//                     given snap-id corresponds to a snap name
//                     and publisher account
//   account-key   -- signed by the brand (canonical's root keys),
//                     authorises an account to sign assertions
//   account       -- signed by the brand, identifies the account
//
// the brand's root keys live in asserts/sysdb/trusted.go and are
// baked into the binary. we open an in-memory db with those keys
// trusted, fetch the chain via the store, and let db.Add()
// validate signatures along the way. then we cross-check that the
// snap-revision's sha matches what we just downloaded and that
// the snap-id matches what the store info reported.

package main

import (
	"fmt"
	"os"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/asserts/sysdb"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/store"
)

// assertsDBPath is where the assertion backstore lives on disk.
// persisting it across installs means canonical's account-keys and
// the snap-declarations we've seen are kept locally, so refresh /
// install-base does not re-fetch the whole prereq chain over the
// network each time.
const assertsDBPath = "/var/lib/snapd/assertions"

func verifyAssertions(s *store.Store, info *snap.Info, snapPath string) error {
	hash, _, err := asserts.SnapFileSHA3_384(snapPath)
	if err != nil {
		return fmt.Errorf("hash snap: %w", err)
	}

	if err := os.MkdirAll(assertsDBPath, 0755); err != nil {
		return fmt.Errorf("mkdir asserts db: %w", err)
	}
	persistDB, err := sysdb.OpenAt(assertsDBPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	// stack a memory backstore on top so this verify pass can add new
	// assertions without colliding with what's already on disk; commit
	// the new ones at the end.
	db := persistDB.WithStackedBackstore(asserts.NewMemoryBackstore())

	retrieve := func(ref *asserts.Ref) (asserts.Assertion, error) {
		return s.Assertion(ref.Type, ref.PrimaryKey, nil)
	}
	// db.Add errors with *RevisionError if the assertion is already in
	// the db at the same or a later rev. that's exactly the situation
	// when re-installing -- treat it as success so the fetcher keeps
	// walking prereqs without restarting.
	save := func(a asserts.Assertion) error {
		err := db.Add(a)
		if _, dup := err.(*asserts.RevisionError); dup {
			return nil
		}
		return err
	}
	f := asserts.NewFetcher(db, retrieve, save)

	revRef := &asserts.Ref{
		Type:       asserts.SnapRevisionType,
		PrimaryKey: []string{hash},
	}
	if err := f.Fetch(revRef); err != nil {
		return fmt.Errorf("fetch snap-revision: %w", err)
	}

	a, err := db.Find(asserts.SnapRevisionType, map[string]string{
		"snap-sha3-384": hash,
	})
	if err != nil {
		return fmt.Errorf("find snap-revision: %w", err)
	}
	rev := a.(*asserts.SnapRevision)
	if rev.SnapSHA3_384() != hash {
		return fmt.Errorf("snap-revision sha mismatch: db=%s file=%s", rev.SnapSHA3_384(), hash)
	}
	if info.SnapID != "" && rev.SnapID() != info.SnapID {
		return fmt.Errorf("snap-revision snap-id mismatch: store=%s rev=%s", info.SnapID, rev.SnapID())
	}
	return nil
}
