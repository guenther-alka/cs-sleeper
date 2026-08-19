//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// startDaemon re-executes this binary in daemon mode, detached into its own
// session so it survives the parent's exit.
func startDaemon(configPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "daemon", "--config", configPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

// stopDaemon signals the running daemon (per the pid file) to exit.
func stopDaemon(pidFile string) error {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return err
	}
	if pid <= 0 || !processAlive(pid) {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}
