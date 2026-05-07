// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2025 Canonical Ltd
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

package osutil

import (
	"fmt"
	"os"
	"strings"
)

// MustRunInContainer exits with a loud error if the process is not running
// inside a container. the no-systemd prototype bypasses confinement entirely
// and must never run on a real host.
func MustRunInContainer() {
	if isContainer() {
		return
	}
	fmt.Fprint(os.Stderr, `
!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
!!  ERROR: this binary must run inside a container.                           !!
!!                                                                           !!
!!  it bypasses confinement entirely — no apparmor, no seccomp, no mount      !!
!!  namespace, no cgroup isolation. running on a real host is unsafe and      !!
!!  will modify system state.                                                !!
!!                                                                           !!
!!  use the docker image instead:                                            !!
!!      make docker && make demo                                             !!
!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
`)
	os.Exit(1)
}

func isContainer() bool {
	// docker drops /.dockerenv, podman drops /run/.containerenv. on cgroup
	// v2 hosts /proc/1/cgroup is just "0::/" with no identifying marker,
	// so the file probes are the reliable signal.
	for _, p := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	s := string(data)
	for _, marker := range []string{"docker", "lxc", "kubepods", "libpod", "containerd"} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}
