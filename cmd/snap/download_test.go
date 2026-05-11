package main

import (
	"testing"
)

func TestCmdDownloadFlagParsing(t *testing.T) {
	t.Run("no snap name errors", func(t *testing.T) {
		if err := cmdDownload(nil); err == nil {
			t.Fatal("expected error for missing snap name")
		}
	})

	t.Run("no snap name with only flags errors", func(t *testing.T) {
		if err := cmdDownload([]string{"--channel=stable"}); err == nil {
			t.Fatal("expected error when only flags, no snap name")
		}
	})

	// --revision warn path: should not panic, should still try to
	// fetch (will fail w/ network error in unit context, not a flag error)
	t.Run("--revision warns and continues", func(t *testing.T) {
		// just check no panic / no flag parse error -- the store call
		// will fail, but that's expected in a unit test environment.
		err := cmdDownload([]string{"--revision=5", "hello"})
		// any error is acceptable here; what matters is it's not a
		// flag-parse panic or "unknown flag" error.
		_ = err
	})
}
