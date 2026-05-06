// -*- Mode: Go; indent-tabs-mode: t -*-

package devicestate

import (
	"github.com/snapcore/snapd/overlord/state"
)

// VolumeStructureWithKeyslots is a stub type.
type VolumeStructureWithKeyslots struct{}

// GetVolumeStructuresWithKeyslots is stubbed - FDE is out of scope.
func GetVolumeStructuresWithKeyslots(st *state.State) ([]VolumeStructureWithKeyslots, error) {
	return nil, nil
}
