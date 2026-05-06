// -*- Mode: Go; indent-tabs-mode: t -*-

package wrappers

import (
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/timings"
)

// DisabledServices is a stub - services not supported in this build.
type DisabledServices struct {
	SystemServices []string
	UserServices   map[int][]string
}

// QueryDisabledServices is a stub.
func QueryDisabledServices(info *snap.Info, pb interface{}) (*DisabledServices, error) {
	return &DisabledServices{}, nil
}

// SnapServiceOptions is a stub.
type SnapServiceOptions struct {
	VitalityRank int
}

// StartServicesOptions is a stub.
type StartServicesOptions struct {
	Enable bool
}

// EnsureSnapServicesOptions is a stub.
type EnsureSnapServicesOptions struct {
	Preseeding             bool
	RequireMountedSnapdSnap bool
}

// AddSnapdSnapServicesOptions is a stub.
type AddSnapdSnapServicesOptions struct{Preseeding bool}

// StopServices is a stub.
func StopServices(apps []*snap.AppInfo, removedSvcs map[string]*snap.AppInfo, disabledSvcs *DisabledServices, reason snap.ServiceStopReason, inter interface{}, tm timings.Measurer) error {
	return nil
}

// StartServices is a stub.
func StartServices(apps []*snap.AppInfo, disabledSvcs *DisabledServices, opts *StartServicesOptions, inter interface{}, tm timings.Measurer) error {
	return nil
}

// AddSnapdSnapServices is a stub.
func AddSnapdSnapServices(s *snap.Info, opts *AddSnapdSnapServicesOptions, inter interface{}) error {
	return nil
}

// EnsureSnapServices is a stub.
func EnsureSnapServices(snapInfo map[*snap.Info]*SnapServiceOptions, opts *EnsureSnapServicesOptions, inter interface{}, pb interface{}) error {
	return nil
}

// RemoveSnapServices is a stub.
func RemoveSnapServices(s *snap.Info, inter interface{}) error {
	return nil
}

// AddSnapDBusActivationFiles is a stub.
func AddSnapDBusActivationFiles(s *snap.Info) error {
	return nil
}

// RemoveSnapDBusActivationFiles is a stub.
func RemoveSnapDBusActivationFiles(s *snap.Info) error {
	return nil
}

// RemoveSnapdSnapServicesOnCore is a stub.
func RemoveSnapdSnapServicesOnCore(s *snap.Info, inter interface{}) error {
	return nil
}
