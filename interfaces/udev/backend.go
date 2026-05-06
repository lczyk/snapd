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

// Package udev implements integration between snapd, udev and
// snap-confine around tagging character and block devices so that they
// can be accessed by applications.
//
// TODO: Document this better
package udev

import (
	"fmt"
	"path/filepath"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/interfaces"
	"github.com/snapcore/snapd/sandbox/cgroup"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/timings"
)

// Backend is responsible for maintaining udev rules.
type Backend struct {
	preseed     bool
	isContainer bool
}

// Initialize does nothing.
func (b *Backend) Initialize(opts *interfaces.SecurityBackendOptions) error {
	if opts != nil && opts.Preseed {
		b.preseed = true
	}
	// Since snapd 2.68 the udev backend, responsible for writing udev rules to
	// /etc/udev/rules.d and for calling udevadm control --reload-rules, as
	// well as udevadm trigger (with a number of options), is no longer enabled
	// in containers. System administrators retain ability to manage access to
	// real devices at the container level.
	//
	// For context:
	//
	// In Linux, devices are _not_ namespace aware so if a device is accessible
	// in the container (and the container manager has allowed such access)
	// then allow snaps to freely poke the device subject to still-enforced
	// apparmor rules. In "traditional" containers such as docker or podman,
	// where using systemd is unusual and unsupported this doesn't change
	// anything. In system containers such as lxd and incus users may, with or
	// without understanding the consequences, switch the container to
	// privileged mode. In this mode udev does start inside the container, but
	// actively configures devices on the host with undesirable consequences.
	//
	// But we want the backend active when preseeding so preseeded images
	// actually have the files in /var/lib/snapd/cgroup.
	b.isContainer = true
	return nil
}

// Name returns the name of the backend.
func (b *Backend) Name() interfaces.SecuritySystem {
	return interfaces.SecurityUDev
}

func (b *Backend) Prepare(_ *interfaces.SnapAppSet) error {
	// No preparation required.
	return nil
}

// snapRulesFileName returns the path of the snap udev rules file.
func snapRulesFilePath(snapName string) string {
	rulesFileName := fmt.Sprintf("70-%s.rules", snap.SecurityTag(snapName))
	return filepath.Join(dirs.SnapUdevRulesDir, rulesFileName)
}

// Setup creates udev rules specific to a given snap.
// If any of the rules are changed or removed then udev database is reloaded.
//
// UDev has no concept of a complain mode so confinement options are ignored.
//
// If the method fails it should be re-tried (with a sensible strategy) by the caller.
func (b *Backend) Setup(appSet *interfaces.SnapAppSet, opts interfaces.ConfinementOptions, sctx interfaces.SetupContext, repo *interfaces.Repository, tm timings.Measurer) error {
	// stub: not available in no-systemd prototype
	return nil
}

// Remove removes udev rules specific to a given snap.
// If any of the rules are removed then udev database is reloaded.
//
// This method should be called after removing a snap.
//
// If the method fails it should be re-tried (with a sensible strategy) by the caller.
func (b *Backend) Remove(snapName string) error {
	// stub: not available in no-systemd prototype
	return nil
}

func (b *Backend) deriveContent(spec *Specification) (content []string) {
	content = append(content, spec.Snippets()...)
	return content
}

func (b *Backend) NewSpecification(appSet *interfaces.SnapAppSet, opts interfaces.ConfinementOptions) interfaces.Specification {
	return &Specification{appSet: appSet}
}

// SandboxFeatures returns the list of features supported by snapd for mediating access to kernel devices.
func (b *Backend) SandboxFeatures() []string {
	commonFeatures := []string{
		"tagging",          /* Tagging dynamically associates new devices with specific snaps */
		"device-filtering", /* Snapd can limit device access for each snap */
	}

	if cgroup.IsUnified() {
		return append(commonFeatures,
			"device-cgroup-v2", /* Snapd creates a device group (v2) for each snap */
		)
	} else {
		return append(commonFeatures,
			"device-cgroup-v1", /* Snapd creates a device group (v1) for each snap */
		)
	}
}
