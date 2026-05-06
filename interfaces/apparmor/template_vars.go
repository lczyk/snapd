// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016 Canonical Ltd
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

package apparmor

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/snapcore/snapd/snap"
)

func safePath(s string) string {
	const allowed = `abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789`
	var buf bytes.Buffer
	for _, c := range []byte(s) {
		if strings.IndexByte(allowed, c) >= 0 {
			fmt.Fprintf(&buf, "%c", c)
		} else {
			fmt.Fprintf(&buf, "_%02x", c)
		}
	}
	return buf.String()
}

// templateVariables returns text defining apparmor variables that can be used
// in the apparmor template and by apparmor snippets.
func templateVariables(info *snap.Info, securityTag string, cmdName string) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# This is a snap name without the instance key\n")
	fmt.Fprintf(&buf, "@{SNAP_NAME}=\"%s\"\n", info.SnapName())
	fmt.Fprintf(&buf, "# This is a snap name with instance key\n")
	fmt.Fprintf(&buf, "@{SNAP_INSTANCE_NAME}=\"%s\"\n", info.InstanceName())
	fmt.Fprintf(&buf, "@{SNAP_INSTANCE_DESKTOP}=\"%s\"\n", info.DesktopPrefix())
	fmt.Fprintf(&buf, "@{SNAP_COMMAND_NAME}=\"%s\"\n", cmdName)
	fmt.Fprintf(&buf, "@{SNAP_REVISION}=\"%s\"\n", info.Revision)
	fmt.Fprintf(&buf, "@{PROFILE_DBUS}=\"%s\"\n",
		safePath(securityTag))
	fmt.Fprintf(&buf, "@{INSTALL_DIR}=\"/{,var/lib/snapd/}snap\"")
	return buf.String()
}
