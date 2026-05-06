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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/progress"
	"github.com/snapcore/snapd/snap"
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

func mountPidFile(mountDir string) string {
	return filepath.Join(dirs.SnapRunDir, "mounts", strings.ReplaceAll(mountDir, "/", "-")+".pid")
}

func addMountUnit(c snap.ContainerPlaceInfo, mountFlags MountUnitFlags) error {
	squashfsPath := dirs.StripRootDir(c.MountFile())
	whereDir := dirs.StripRootDir(c.MountDir())

	if err := os.MkdirAll(whereDir, 0755); err != nil {
		return err
	}

	cmd := exec.Command("squashfuse", squashfsPath, whereDir)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot mount snap with squashfuse: %v", err)
	}

	pidDir := filepath.Join(dirs.SnapRunDir, "mounts")
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return err
	}

	pidStr := fmt.Sprintf("%d", cmd.Process.Pid)
	if err := os.WriteFile(mountPidFile(whereDir), []byte(pidStr), 0644); err != nil {
		return err
	}

	logger.Debugf("squashfuse mounted %s -> %s (pid %d)", squashfsPath, whereDir, cmd.Process.Pid)
	return nil
}

func removeMountUnit(mountDir string, meter progress.Meter) error {
	pidFile := mountPidFile(mountDir)
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("cannot find mount pid for %s: %v", mountDir, err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("invalid pid file %s: %v", pidFile, err)
	}

	proc, err := os.FindProcess(pid)
	if err == nil {
		proc.Signal(syscall.SIGTERM)
	}
	os.Remove(pidFile)

	exec.Command("fusermount", "-u", mountDir).Run()

	return nil
}

func (b Backend) RemoveContainerMountUnits(s snap.ContainerPlaceInfo, meter progress.Meter) error {
	pidDir := filepath.Join(dirs.SnapRunDir, "mounts")
	entries, err := os.ReadDir(pidDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	prefix := strings.ReplaceAll(s.MountDir(), "/", "-")
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			pidFile := filepath.Join(pidDir, entry.Name())
			data, err := os.ReadFile(pidFile)
			if err != nil {
				continue
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				continue
			}
			proc, err := os.FindProcess(pid)
			if err == nil {
				proc.Signal(syscall.SIGTERM)
			}
			os.Remove(pidFile)
		}
	}
	return nil
}
