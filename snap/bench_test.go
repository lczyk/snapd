package snap

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	SanitizePlugsSlots = func(snapInfo *Info) {}
	os.Exit(m.Run())
}
