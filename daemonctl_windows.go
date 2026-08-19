//go:build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/xpath"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// startDaemon re-executes this binary in daemon mode as a detached process.
func startDaemon(configPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "daemon", "--config", configPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | createNewProcessGroup,
	}
	return cmd.Start()
}

// stopDaemon terminates the running daemon (per the pid file) and its children.
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
	taskkill := xpath.Resolve("taskkill", `C:\Windows\System32\taskkill.exe`)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, taskkill, "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}
