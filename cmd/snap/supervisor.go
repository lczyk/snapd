// snap-super mode: entered when snap is invoked with
// --secret-daemon-mode as the first argument. manages one daemon
// from a snap's snap.yaml: forks it, supervises restarts with
// exponential backoff, serves IPC commands over a unix socket.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/snapcore/snapd/snap"
)

// runSupervisor is the snap-super entry point. target is "snapname.svcname".
func runSupervisor(target string) error {
	snapName, svcName, err := parseServiceTarget(target)
	if err != nil {
		return err
	}

	mountDir := filepath.Join(snapMountDir, snapName, "current")
	yamlBytes, err := os.ReadFile(filepath.Join(mountDir, "meta", "snap.yaml"))
	if err != nil {
		return fmt.Errorf("read snap.yaml: %w", err)
	}
	info, err := snap.InfoFromSnapYaml(yamlBytes)
	if err != nil {
		return fmt.Errorf("parse snap.yaml: %w", err)
	}
	app, ok := info.Apps[svcName]
	if !ok {
		return fmt.Errorf("app %q not found in snap %q", svcName, snapName)
	}
	if !app.IsService() {
		return fmt.Errorf("app %q in snap %q is not a daemon", svcName, snapName)
	}

	// resolve revision symlink so env vars are concrete paths
	revLink, err := os.Readlink(mountDir)
	if err != nil {
		return fmt.Errorf("readlink current: %w", err)
	}
	revStr := filepath.Base(revLink)
	concreteMount := filepath.Join(snapMountDir, snapName, revStr)

	// write our own pid file
	if err := os.MkdirAll(supervisorDir, 0755); err != nil {
		return fmt.Errorf("mkdir supervisors: %w", err)
	}
	pidPath := supervisorPidPath(snapName, svcName)
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	defer os.Remove(pidPath)

	// open log file; writes go to log + container stdout
	if err := os.MkdirAll(daemonLogDir, 0755); err != nil {
		return fmt.Errorf("mkdir log dir: %w", err)
	}
	logFile, err := os.OpenFile(supervisorLogPath(snapName, svcName),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()
	logOut := io.MultiWriter(logFile, os.Stdout)

	// open unix socket for IPC
	sockPath := supervisorSockPath(snapName, svcName)
	_ = os.Remove(sockPath)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", sockPath, err)
	}
	defer func() {
		ln.Close()
		os.Remove(sockPath)
	}()

	restartCond := resolveRestartCond(app.RestartCond, snapName, svcName)

	stopCh := make(chan struct{})
	restartCh := make(chan struct{}, 1)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleIPCConn(conn, stopCh, restartCh)
		}
	}()

	return supervisorLoop(info, app, concreteMount, snapName, svcName, revStr, restartCond, logFile, logOut, stopCh, restartCh)
}

func supervisorLoop(
	info *snap.Info,
	app *snap.AppInfo,
	concreteMount, snapName, svcName, revStr string,
	restartCond snap.RestartCondition,
	logFile *os.File,
	logOut io.Writer,
	stopCh <-chan struct{},
	restartCh <-chan struct{},
) error {
	const (
		maxBackoff      = 30 * time.Second
		stableThreshold = 30 * time.Second
	)
	backoff := time.Second

	for {
		cmd := buildDaemonCmd(info, app, concreteMount, snapName, revStr)
		// NOTE: use logFile (*os.File) directly, not logOut (io.MultiWriter).
		// with a plain file, exec sets the fd directly and cmd.Wait() returns
		// as soon as the process exits. with an io.Writer, exec creates a pipe
		// + copy goroutine and cmd.Wait() blocks until all processes holding
		// the write end of that pipe exit -- including forked children of the
		// daemon that outlive the main process (e.g. mosquitto forks a child).
		cmd.Stdout = logFile
		cmd.Stderr = logFile

		fmt.Fprintf(logOut, "[snap-super] starting %s.%s\n", snapName, svcName)
		startTime := time.Now()

		if err := cmd.Start(); err != nil {
			fmt.Fprintf(logOut, "[snap-super] start failed: %v\n", err)
			// treat a failed start like a crash for backoff purposes
		} else {
			doneCh := make(chan error, 1)
			go func() { doneCh <- cmd.Wait() }()

			select {
			case <-stopCh:
				return gracefulStop(cmd, doneCh, logOut, snapName, svcName)

			case <-restartCh:
				_ = gracefulStop(cmd, doneCh, logOut, snapName, svcName)
				fmt.Fprintf(logOut, "[snap-super] restarting %s.%s\n", snapName, svcName)
				backoff = time.Second
				continue

			case exitErr := <-doneCh:
				uptime := time.Since(startTime)
				code := exitCode(exitErr)
				fmt.Fprintf(logOut, "[snap-super] %s.%s exited (code=%d, uptime=%s)\n",
					snapName, svcName, code, uptime.Round(time.Second))

				if !shouldRestart(restartCond, code) {
					fmt.Fprintf(logOut, "[snap-super] not restarting (%s)\n", restartCond)
					return nil
				}
				if uptime >= stableThreshold {
					backoff = time.Second
				}
			}
		}

		fmt.Fprintf(logOut, "[snap-super] restarting in %s\n", backoff)
		select {
		case <-time.After(backoff):
		case <-stopCh:
			return nil
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func gracefulStop(cmd *exec.Cmd, doneCh <-chan error, logOut io.Writer, snapName, svcName string) error {
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-doneCh:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-doneCh
	}
	fmt.Fprintf(logOut, "[snap-super] stopped %s.%s\n", snapName, svcName)
	return nil
}

func handleIPCConn(conn net.Conn, stopCh chan struct{}, restartCh chan struct{}) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	var req ipcRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		json.NewEncoder(conn).Encode(ipcResponse{OK: false, Error: err.Error()})
		return
	}

	var resp ipcResponse
	switch req.Command {
	case "stop":
		resp = ipcResponse{OK: true}
		json.NewEncoder(conn).Encode(resp)
		// signal the supervisor loop after responding so the client
		// gets its ack before we start tearing down.
		select {
		case <-stopCh:
			// already closing
		default:
			close(stopCh)
		}
	case "restart":
		select {
		case restartCh <- struct{}{}:
			resp = ipcResponse{OK: true}
		default:
			resp = ipcResponse{OK: false, Error: "restart already pending"}
		}
		json.NewEncoder(conn).Encode(resp)
	case "status":
		json.NewEncoder(conn).Encode(ipcResponse{OK: true})
	default:
		json.NewEncoder(conn).Encode(ipcResponse{OK: false, Error: "unknown command: " + req.Command})
	}
}

func buildDaemonCmd(info *snap.Info, app *snap.AppInfo, concreteMount, snapName, revStr string) *exec.Cmd {
	cmdPath := filepath.Join(concreteMount, app.Command)
	argv := []string{cmdPath}

	if len(app.CommandChain) > 0 {
		chain := make([]string, 0, len(app.CommandChain))
		for _, e := range app.CommandChain {
			chain = append(chain, filepath.Join(concreteMount, e))
		}
		argv = append(chain, argv...)
	}

	envMap := buildRunEnv(info, app, concreteMount, snapName, revStr)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = mapToEnv(envMap)
	return cmd
}

// resolveRestartCond maps the snap.yaml restart field to a supported
// RestartCondition, warning and falling back for unsupported values.
func resolveRestartCond(cond snap.RestartCondition, snapName, svcName string) snap.RestartCondition {
	switch cond {
	case snap.RestartAlways, snap.RestartNever, snap.RestartOnFailure, snap.RestartOnSuccess:
		return cond
	case "": // snap.yaml omitted restart: -- real snap defaults to on-failure
		return snap.RestartOnFailure
	default:
		fmt.Fprintf(os.Stderr,
			"warning: snap %s daemon %s uses restart %q which is not supported; using 'on-failure'\n",
			snapName, svcName, cond)
		return snap.RestartOnFailure
	}
}

func shouldRestart(cond snap.RestartCondition, code int) bool {
	switch cond {
	case snap.RestartAlways:
		return true
	case snap.RestartNever:
		return false
	case snap.RestartOnFailure:
		return code != 0
	case snap.RestartOnSuccess:
		return code == 0
	default:
		return false
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}
