// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016-2022 Canonical Ltd
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

package backends

import (
	"github.com/snapcore/snapd/interfaces"
	"github.com/snapcore/snapd/interfaces/apparmor"
	"github.com/snapcore/snapd/interfaces/configfiles"
	"github.com/snapcore/snapd/interfaces/kmod"
	"github.com/snapcore/snapd/interfaces/ldconfig"
	"github.com/snapcore/snapd/interfaces/mount"
	"github.com/snapcore/snapd/interfaces/polkit"
	"github.com/snapcore/snapd/interfaces/seccomp"
	"github.com/snapcore/snapd/interfaces/symlinks"
	"github.com/snapcore/snapd/interfaces/udev"
	"github.com/snapcore/snapd/logger"
	apparmor_sandbox "github.com/snapcore/snapd/sandbox/apparmor"
)

// All returns a set of all available security backends.
func All() []interfaces.SecurityBackend {
	all := []interfaces.SecurityBackend{
		&seccomp.Backend{},
		&udev.Backend{},
		&mount.Backend{},
		&kmod.Backend{},
		&polkit.Backend{},
		&ldconfig.Backend{},
		&configfiles.Backend{},
		&symlinks.Backend{},
	}

	logger.Noticef("AppArmor status: %s\n", apparmor_sandbox.Summary())

	switch apparmor_sandbox.ProbedLevel() {
	case apparmor_sandbox.Partial, apparmor_sandbox.Full:
		all = append(all, &apparmor.Backend{})
	}
	return all
}
