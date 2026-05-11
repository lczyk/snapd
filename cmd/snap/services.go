// snap-cli service management commands: services, start, stop,
// restart, logs. enable/disable are no-ops with a warning.

package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"
)

func cmdServices(_ []string) error {
	entries, err := os.ReadDir("/snap")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	type row struct {
		service string
		active  bool
	}
	var rows []row

	for _, e := range entries {
		if !e.IsDir() || e.Name() == "bin" {
			continue
		}
		snapName := e.Name()
		daemons, err := declaredDaemons(snapName)
		if err != nil {
			continue
		}
		for _, app := range daemons {
			rows = append(rows, row{
				service: snapName + "." + app.Name,
				active:  isSupervisorRunning(snapName, app.Name),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].service < rows[j].service })

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Service\tStartup\tCurrent")
	for _, r := range rows {
		status := "inactive"
		if r.active {
			status = "active"
		}
		fmt.Fprintf(w, "%s\tenabled\t%s\n", r.service, status)
	}
	return w.Flush()
}

func cmdStart(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("start needs a snap[.service] target")
	}
	for _, target := range args {
		snapName, svcName := resolveServiceTarget(target)
		if svcName != "" {
			if err := startOne(snapName, svcName); err != nil {
				return err
			}
		} else {
			daemons, err := declaredDaemons(snapName)
			if err != nil {
				return fmt.Errorf("read daemons for %s: %w", snapName, err)
			}
			for _, app := range daemons {
				if err := startOne(snapName, app.Name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func startOne(snapName, svcName string) error {
	if isSupervisorRunning(snapName, svcName) {
		fmt.Printf("%s.%s is already running\n", snapName, svcName)
		return nil
	}
	if err := startSupervisor(snapName, svcName); err != nil {
		return fmt.Errorf("start %s.%s: %w", snapName, svcName, err)
	}
	fmt.Printf("started %s.%s\n", snapName, svcName)
	return nil
}

func cmdStop(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("stop needs a snap[.service] target")
	}
	for _, target := range args {
		snapName, svcName := resolveServiceTarget(target)
		if svcName != "" {
			if err := stopOne(snapName, svcName); err != nil {
				return err
			}
		} else {
			daemons, err := declaredDaemons(snapName)
			if err != nil {
				return fmt.Errorf("read daemons for %s: %w", snapName, err)
			}
			for _, app := range daemons {
				if !isSupervisorRunning(snapName, app.Name) {
					continue
				}
				if err := stopOne(snapName, app.Name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func cmdRestart(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("restart needs a snap[.service] target")
	}
	for _, target := range args {
		snapName, svcName := resolveServiceTarget(target)
		if svcName != "" {
			if err := restartOne(snapName, svcName); err != nil {
				return err
			}
		} else {
			daemons, err := declaredDaemons(snapName)
			if err != nil {
				return fmt.Errorf("read daemons for %s: %w", snapName, err)
			}
			for _, app := range daemons {
				if err := restartOne(snapName, app.Name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func restartOne(snapName, svcName string) error {
	if !isSupervisorRunning(snapName, svcName) {
		// not running -- just start it
		return startOne(snapName, svcName)
	}
	if err := sendSupervisorCommand(snapName, svcName, "restart", 10*time.Second); err != nil {
		return fmt.Errorf("restart %s.%s: %w", snapName, svcName, err)
	}
	fmt.Printf("restarted %s.%s\n", snapName, svcName)
	return nil
}

func cmdLogs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("logs needs a snap.service target")
	}
	snapName, svcName, err := parseServiceTarget(args[0])
	if err != nil {
		return err
	}
	logPath := supervisorLogPath(snapName, svcName)
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no logs for %s.%s", snapName, svcName)
		}
		return err
	}
	os.Stdout.Write(data)
	return nil
}

func cmdEnableDisable(verb string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s needs a snap[.service] target", verb)
	}
	fmt.Fprintf(os.Stderr,
		"warning: snap %s is not supported in this build (no boot persistence); ignoring\n", verb)
	return nil
}
