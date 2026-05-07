// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2016 Canonical Ltd
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

package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/progress"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/snap/squashfs"
)

// MountUnitFlags contains flags that modify behavior of addMountUnit
type MountUnitFlags struct {
	// PreventRestartIfModified is set if we do not want to restart the
	// mount unit if even though it was modified
	PreventRestartIfModified bool
	// StartBeforeDriversLoad is set if the unit is needed before
	// udevd starts to run rules
	StartBeforeDriversLoad bool
}

func mountMarkerFile(mountDir string) string {
	return filepath.Join(dirs.SnapRunDir, "mounts", strings.ReplaceAll(mountDir, "/", "-")+".marker")
}

func addMountUnit(c snap.ContainerPlaceInfo, mountFlags MountUnitFlags) error {
	squashfsPath := dirs.StripRootDir(c.MountFile())
	whereDir := dirs.StripRootDir(c.MountDir())

	if err := os.MkdirAll(whereDir, 0755); err != nil {
		return err
	}

	// extract directly to disk -- no FUSE, no privileges needed
	sn := squashfs.New(squashfsPath)
	if err := sn.Unpack("*", whereDir); err != nil {
		return fmt.Errorf("cannot extract snap: %v", err)
	}

	markerDir := filepath.Join(dirs.SnapRunDir, "mounts")
	if err := os.MkdirAll(markerDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(mountMarkerFile(whereDir), []byte("ok"), 0644); err != nil {
		return err
	}

	logger.Debugf("unsquashfs extracted %s -> %s", squashfsPath, whereDir)
	return nil
}

func removeMountUnit(mountDir string, meter progress.Meter) error {
	marker := mountMarkerFile(mountDir)
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		return fmt.Errorf("cannot find mount marker for %s: %v", mountDir, err)
	}
	os.Remove(marker)

	// remove extracted contents
	if err := os.RemoveAll(mountDir); err != nil {
		return err
	}

	return nil
}

func (b Backend) RemoveContainerMountUnits(s snap.ContainerPlaceInfo, meter progress.Meter) error {
	markerDir := filepath.Join(dirs.SnapRunDir, "mounts")
	entries, err := os.ReadDir(markerDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	prefix := strings.ReplaceAll(s.MountDir(), "/", "-")
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			os.Remove(filepath.Join(markerDir, entry.Name()))
		}
	}
	return nil
}
