// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2021-2024 Canonical Ltd
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

// Package polkit implements interaction between snapd and polkit.
//
// Snapd installs polkitd policy files on behalf of snaps that
// describe administrative actions they can perform on behalf of
// clients.
//
// The policy files are XML files whose format is described here:
// https://www.freedesktop.org/software/polkit/docs/latest/polkit.8.html#polkit-declaring-actions
package polkit

import (
	"fmt"

	"github.com/snapcore/snapd/interfaces"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/timings"
)

func polkitPolicyName(snapName, nameSuffix string) string {
	return snap.ScopedSecurityTag(snapName, "interface", nameSuffix) + ".policy"
}

func polkitRuleName(snapName, nameSuffix string) string {
	// 70-<security-tag>.<file-name>.rules
	return fmt.Sprintf("70-%s.%s.rules", snap.SecurityTag(snapName), nameSuffix)
}

// Backend is responsible for maintaining polkitd policy files.
type Backend struct{}

// Initialize does nothing.
func (b *Backend) Initialize(*interfaces.SecurityBackendOptions) error {
	return nil
}

// Name returns the name of the backend.
func (b *Backend) Name() interfaces.SecuritySystem {
	return interfaces.SecurityPolkit
}

func (b *Backend) Prepare(_ *interfaces.SnapAppSet) error {
	// No preparation required.
	return nil
}

// Setup installs the polkit policy and rule files specific to a given snap.
//
// Polkit has no concept of a complain mode so confinment type is ignored.
func (b *Backend) Setup(appSet *interfaces.SnapAppSet, opts interfaces.ConfinementOptions, sctx interfaces.SetupContext, repo *interfaces.Repository, tm timings.Measurer) error {
	// stub: not available in no-systemd prototype
	return nil
}

// Remove removes polkit policy and rule files of a given snap.
//
// This method should be called after removing a snap.
func (b *Backend) Remove(snapName string) error {
	// stub: not available in no-systemd prototype
	return nil
}

// derivePoliciesContent combines polkit policies collected from all the interfaces
// affecting a given snap into a content map applicable to EnsureDirState.
func derivePoliciesContent(spec *Specification, appSet *interfaces.SnapAppSet) map[string]osutil.FileState {
	policies := spec.Policies()
	if len(policies) == 0 {
		return nil
	}
	content := make(map[string]osutil.FileState, len(policies)+1)
	for nameSuffix, policyContent := range policies {
		filename := polkitPolicyName(appSet.InstanceName(), nameSuffix)
		content[filename] = &osutil.MemoryFileState{
			Content: policyContent,
			Mode:    0644,
		}
	}
	return content
}

// deriveRulesContent combines polkit rules collected from all the interfaces
// affecting a given snap into a content map applicable to EnsureDirState.
func deriveRulesContent(spec *Specification, appSet *interfaces.SnapAppSet) map[string]osutil.FileState {
	rules := spec.Rules()
	if len(rules) == 0 {
		return nil
	}
	content := make(map[string]osutil.FileState, len(rules)+1)
	for nameSuffix, ruleContent := range rules {
		filename := polkitRuleName(appSet.InstanceName(), nameSuffix)
		content[filename] = &osutil.MemoryFileState{
			Content: ruleContent,
			Mode:    0644,
		}
	}
	return content
}

func (b *Backend) NewSpecification(*interfaces.SnapAppSet, interfaces.ConfinementOptions) interfaces.Specification {
	return &Specification{}
}

// SandboxFeatures returns list of features supported by snapd for polkit policy.
func (b *Backend) SandboxFeatures() []string {
	return nil
}
