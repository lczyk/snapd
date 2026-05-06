// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016-2024 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

// Package mount implements mounts that get mapped into the snap
//
// Snappy creates fstab like  configuration files that describe what
// directories from the system or from other snaps should get mapped
// into the snap.
//
// Each fstab like file looks like a regular fstab entry:
//
//	/src/dir /dst/dir none bind 0 0
//	/src/dir /dst/dir none bind,rw 0 0
//
// but only bind mounts are supported
package mount

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/snapcore/snapd/cmd/snaplock"
	"github.com/snapcore/snapd/interfaces"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/sandbox/cgroup"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/timings"
)

// Backend is responsible for maintaining mount files for snap-confine
type Backend struct{}

// Initialize does nothing.
func (b *Backend) Initialize(*interfaces.SecurityBackendOptions) error {
	return nil
}

// Name returns the name of the backend.
func (b *Backend) Name() interfaces.SecuritySystem {
	return interfaces.SecurityMount
}

func (b *Backend) Prepare(_ *interfaces.SnapAppSet) error {
	// no special preparation required
	return nil
}

const (
	// DelayedConsumerMountNsUpdate identifies an effect of updating the mount
	// namespace of a connected consumer.
	DelayedConsumerMountNsUpdate = interfaces.DelayedEffect("delayed-consumer-mount-ns-update")
)

// Setup creates mount mount profile files specific to a given snap.
func (b *Backend) Setup(appSet *interfaces.SnapAppSet, opts interfaces.ConfinementOptions, sctx interfaces.SetupContext, repo *interfaces.Repository, tm timings.Measurer) error {
	// stub: not available in no-systemd prototype
	return nil
}

// updateOrDiscard attempts to update the mount namespace for a snap, and if
// that fails, tries to discard the namespace (unless the snap has enduring daemons).
func (b *Backend) updateOrDiscard(snapName string, snapInfo *snap.Info) error {
	logger.Debugf("update or discard mount ns for snap %v", snapInfo.InstanceName())

	if err := UpdateSnapNamespace(snapName); err != nil {
		// try to discard the mount namespace but only if there aren't enduring daemons in the snap
		for _, app := range snapInfo.Apps {
			if app.Daemon != "" && app.RefreshMode == "endure" {
				return fmt.Errorf("cannot update mount namespace of snap %q, and cannot discard it because it contains an enduring daemon: %s", snapName, err)
			}
		}
		logger.Noticef("discarding mount namespace of snap %q due update failure: %v", snapName, err)
		// In some snaps, if the layout change from a version to the next by replacing a bind by a symlink,
		// the update can fail. Discarding the namespace allows to solve this.
		if err = DiscardSnapNamespace(snapName); err != nil {
			return fmt.Errorf("cannot discard mount namespace of snap %q when trying to update it: %s", snapName, err)
		}
	}
	return nil
}

// Remove removes mount configuration files of a given snap.
//
// This method should be called after removing a snap.
func (b *Backend) Remove(snapName string) error {
	// stub: not available in no-systemd prototype
	return nil
}

// addMountProfile adds a mount profile with the given name, based on the given entries.
//
// If there are no entries no profile is generated.
func addMountProfile(content map[string]osutil.FileState, fname string, entries []osutil.MountEntry) {
	if len(entries) == 0 {
		return
	}
	var buffer bytes.Buffer
	for _, entry := range entries {
		fmt.Fprintf(&buffer, "%s\n", entry)
	}
	content[fname] = &osutil.MemoryFileState{Content: buffer.Bytes(), Mode: 0644}
}

// deriveContent computes .fstab tables based on requests made to the specification.
func deriveContent(spec *Specification, snapInfo *snap.Info) map[string]osutil.FileState {
	content := make(map[string]osutil.FileState, 2)
	snapName := snapInfo.InstanceName()
	// Add the per-snap fstab file.
	// This file is read by snap-update-ns in the global pass.
	addMountProfile(content, fmt.Sprintf("snap.%s.fstab", snapName), spec.MountEntries())
	// Add the per-snap user-fstab file.
	// This file will be read by snap-update-ns in the per-user pass.
	addMountProfile(content, fmt.Sprintf("snap.%s.user-fstab", snapName), spec.UserMountEntries())
	return content
}

// NewSpecification returns a new mount specification.
func (b *Backend) NewSpecification(*interfaces.SnapAppSet, interfaces.ConfinementOptions) interfaces.Specification {
	return &Specification{}
}

// SandboxFeatures returns the list of features supported by snapd for composing mount namespaces.
func (b *Backend) SandboxFeatures() []string {
	commonFeatures := []string{
		"layouts",                 /* Mount profiles take layout data into account */
		"mount-namespace",         /* Snapd creates a mount namespace for each snap */
		"per-snap-persistency",    /* Per-snap profiles are persisted across invocations */
		"per-snap-profiles",       /* Per-snap profiles allow changing mount namespace of a given snap */
		"per-snap-updates",        /* Changes to per-snap mount profiles are applied instantly */
		"per-snap-user-profiles",  /* Per-snap profiles allow changing mount namespace of a given snap for a given user */
		"stale-base-invalidation", /* Mount namespaces that go stale because base snap changes are automatically invalidated */
	}
	cgroupv1Features := []string{
		"freezer-cgroup-v1", /* Snapd creates a freezer cgroup (v1) for each snap */
	}

	if cgroup.IsUnified() {
		// TODO: update we get feature parity on cgroup v2
		return commonFeatures
	}

	features := append(commonFeatures, cgroupv1Features...)
	return features
}

var _ interfaces.DelayedSideEffectsBackend = (*Backend)(nil)

var runningApplicationsError = errors.New("snap has running applications")

func (b *Backend) ApplyDelayedEffects(appSet *interfaces.SnapAppSet, work []interfaces.DelayedSideEffect, tm timings.Measurer) error {
	seen := map[interfaces.DelayedEffect]bool{}
	var deduped []interfaces.DelayedSideEffect

	// Remove duplicates, in case a snap was connected to multiple providers.
	// The namespace update has a 'global' effect anyway, so it's sufficient to
	// apply it once.
	for _, w := range work {
		if w.ID != DelayedConsumerMountNsUpdate {
			return fmt.Errorf("unexpected effect: %q", w.ID)
		}
		if !seen[w.ID] {
			deduped = append(deduped, w)
			seen[w.ID] = true
		}
	}

	switch {
	case len(deduped) > 1:
		return fmt.Errorf("internal error: expecting at most one delayed effect to apply")
	case len(deduped) == 1:
		snapName := appSet.InstanceName()
		snapInfo := appSet.Info()

		logger.Debugf("setup delayed for %v", snapName)

		// attempt to discard the mount namespace of affected snap if no
		// applications or service of that snap are running
		if err := opportunisticDiscard(appSet); err == nil {
			// namespace was discarded, we're done
			return nil
		} else {
			// could be running applications or other error, carry on and try to
			// execute a regular update
			logger.Noticef("%v", err)
		}

		// Assuming all non-deferred work was done in Setup(), perform only the
		// remaining work, specifically update or discard the mount namespace
		return b.updateOrDiscard(snapName, snapInfo)
	}
	return nil
}

func opportunisticDiscard(appSet *interfaces.SnapAppSet) error {
	instanceName := appSet.InstanceName()
	return snaplock.WithTryLock(instanceName, func() error {
		paths, err := cgroup.InstancePathsOfSnap(instanceName, cgroup.InstancePathsOptions{
			ReturnCGroupPath: true,
		})
		if err != nil {
			return err
		}

		if len(paths) != 0 {
			return fmt.Errorf("cannot discard when %v instances of snap are running: %w",
				len(paths), runningApplicationsError)
		}

		logger.Debugf("no running applications belonging to snap %q, proceeding to discard the snap's mount namespace",
			instanceName)
		if err = DiscardLockedSnapNamespace(instanceName); err != nil {
			return fmt.Errorf("cannot discard mount namespace of snap %q: %w", instanceName, err)
		}

		return nil
	})
}
