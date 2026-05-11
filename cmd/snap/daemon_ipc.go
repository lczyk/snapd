// shared helpers for snap-cli <-> snap-super communication.
// paths, liveness checks, IPC send, double-fork launch.

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/snapcore/snapd/snap"
)

func supervisorPidPath(snapName, svcName string) string {
	return filepath.Join(supervisorDir, snapName+"."+svcName+".pid")
}

func supervisorSockPath(snapName, svcName string) string {
	return filepath.Join(supervisorDir, snapName+"."+svcName+".sock")
}

func supervisorLogPath(snapName, svcName string) string {
	return filepath.Join(daemonLogDir, snapName+"."+svcName+".log")
}

// isSupervisorRunning checks whether a snap-super process is alive for
// the given snap.service pair by reading its pid file and sending
// signal 0.
func isSupervisorRunning(snapName, svcName string) bool {
	data, err := os.ReadFile(supervisorPidPath(snapName, svcName))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

type ipcRequest struct {
	Command string `json:"command"` // "stop", "restart", "status"
}

type ipcResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// sendSupervisorCommand opens the supervisor's unix socket and sends
// a single newline-delimited json command, reading the response.
func sendSupervisorCommand(snapName, svcName, command string, timeout time.Duration) error {
	conn, err := net.DialTimeout("unix", supervisorSockPath(snapName, svcName), timeout)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	if err := json.NewEncoder(conn).Encode(ipcRequest{Command: command}); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	var resp ipcResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("recv: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("supervisor: %s", resp.Error)
	}
	return nil
}

// startSupervisor forks snap-super for snapName.svcName in the
// background. uses Setsid to detach from the current terminal;
// when snap-cli exits snap-super is reparented to bash.
func startSupervisor(snapName, svcName string) error {
	if err := os.MkdirAll(supervisorDir, 0755); err != nil {
		return fmt.Errorf("mkdir supervisors: %w", err)
	}
	if err := os.MkdirAll(daemonLogDir, 0755); err != nil {
		return fmt.Errorf("mkdir log dir: %w", err)
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find self: %w", err)
	}

	cmd := exec.Command(self, "--secret-daemon-mode", snapName+"."+svcName)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// stdin/stdout/stderr left nil -- supervisor opens its own log file
	// and writes directly to os.Stdout there.
	return cmd.Start()
	// no Wait(): snap-super is intentionally long-lived. when snap-cli
	// exits, snap-super is reparented to bash which reaps it on exit.
}

// emergencyKill is the fallback when IPC fails: SIGKILL the supervisor
// via its pid file and clean up the control files. this is an error
// state that should not happen in normal operation.
func emergencyKill(snapName, svcName string) error {
	data, err := os.ReadFile(supervisorPidPath(snapName, svcName))
	if err != nil {
		return fmt.Errorf("read pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("parse pid: %w", err)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	_ = os.Remove(supervisorPidPath(snapName, svcName))
	_ = os.Remove(supervisorSockPath(snapName, svcName))
	return nil
}

// declaredDaemons returns AppInfo entries for all declared daemons in
// the snap's current snap.yaml.
func declaredDaemons(snapName string) ([]*snap.AppInfo, error) {
	yamlPath := filepath.Join(snapMountDir, snapName, "current", "meta", "snap.yaml")
	yamlBytes, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, err
	}
	info, err := snap.InfoFromSnapYaml(yamlBytes)
	if err != nil {
		return nil, err
	}
	var daemons []*snap.AppInfo
	for _, app := range info.Apps {
		if app.IsService() {
			daemons = append(daemons, app)
		}
	}
	return daemons, nil
}

// resolveServiceTarget splits "snapname.svcname" or bare "snapname"
// into its parts. svcName is empty when only the snap name was given.
func resolveServiceTarget(target string) (snapName, svcName string) {
	if before, after, ok := strings.Cut(target, "."); ok {
		return before, after
	}
	return target, ""
}

// parseServiceTarget is like resolveServiceTarget but requires the
// snap.service form.
func parseServiceTarget(target string) (snapName, svcName string, err error) {
	snapName, svcName = resolveServiceTarget(target)
	if svcName == "" {
		return "", "", fmt.Errorf("expected snap.service format, got %q", target)
	}
	return snapName, svcName, nil
}

// startDaemonsForSnap launches a snap-super for each daemon declared
// in the snap's snap.yaml. called at the end of snap install.
func startDaemonsForSnap(info *snap.Info) error {
	for _, app := range info.Apps {
		if !app.IsService() {
			continue
		}
		if err := startSupervisor(info.SnapName(), app.Name); err != nil {
			return fmt.Errorf("start daemon %s.%s: %w", info.SnapName(), app.Name, err)
		}
		fmt.Printf("started daemon %s.%s\n", info.SnapName(), app.Name)
	}
	return nil
}

// stopDaemonsForSnap stops all running daemon supervisors for a snap.
// called at the start of snap remove, before the snap tree is deleted.
// errors are printed as warnings; we proceed with removal regardless.
func stopDaemonsForSnap(snapName string) {
	daemons, err := declaredDaemons(snapName)
	if err != nil {
		return
	}
	for _, app := range daemons {
		if !isSupervisorRunning(snapName, app.Name) {
			continue
		}
		if err := stopOne(snapName, app.Name); err != nil {
			fmt.Fprintf(os.Stderr, "warning: stop %s.%s: %v\n", snapName, app.Name, err)
		}
	}
}

// stopOne sends an IPC stop command to the supervisor for snapName.svcName.
// if IPC fails or times out, falls back to emergencyKill.
func stopOne(snapName, svcName string) error {
	const ipcTimeout = 10 * time.Second
	err := sendSupervisorCommand(snapName, svcName, "stop", ipcTimeout)
	if err == nil {
		fmt.Printf("stopped %s.%s\n", snapName, svcName)
		return nil
	}
	// NOTE: IPC failure is an error state -- supervisor unresponsive.
	fmt.Fprintf(os.Stderr, "warning: IPC stop failed for %s.%s (%v); killing supervisor\n", snapName, svcName, err)
	return emergencyKill(snapName, svcName)
}

// ensureDaemonsRunning starts any declared daemon supervisors for
// snapName that are not currently running. called lazily from cmdRun
// before exec-ing the app, so daemons come up automatically after a
// container restart without requiring an explicit `snap start`.
func ensureDaemonsRunning(snapName string) {
	daemons, err := declaredDaemons(snapName)
	if err != nil || len(daemons) == 0 {
		return
	}
	for _, app := range daemons {
		if isSupervisorRunning(snapName, app.Name) {
			continue
		}
		if err := startSupervisor(snapName, app.Name); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to start daemon %s.%s: %v\n", snapName, app.Name, err)
		}
	}
}
