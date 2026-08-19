package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// enableCmd persists enabled=yes and starts the daemon.
func enableCmd(args []string) {
	fs := flag.NewFlagSet("enable", flag.ExitOnError)
	configPath := fs.String("config", "", "config file (default /opt/csweb-gui/_cfg/cs-sleeper)")
	fs.Parse(args)

	path := *configPath
	if path == "" {
		path = defaultConfigPath()
	}
	if _, err := loadConfig(path); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := setEnabledInFile(path, true); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	cfg, _ := loadConfig(path)
	if daemonRunning(cfg.PidFile) {
		fmt.Println("cs-sleeper enabled (daemon already running)")
		return
	}
	if err := startDaemon(path); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot start daemon: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("cs-sleeper enabled; daemon started")
}

// disableCmd persists enabled=no and stops the daemon.
func disableCmd(args []string) {
	fs := flag.NewFlagSet("disable", flag.ExitOnError)
	configPath := fs.String("config", "", "config file (default /opt/csweb-gui/_cfg/cs-sleeper)")
	fs.Parse(args)

	path := *configPath
	if path == "" {
		path = defaultConfigPath()
	}
	if _, err := loadConfig(path); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := setEnabledInFile(path, false); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	cfg, _ := loadConfig(path)
	if daemonRunning(cfg.PidFile) {
		if err := stopDaemon(cfg.PidFile); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot stop daemon: %v\n", err)
		} else {
			os.Remove(cfg.PidFile)
		}
	}
	fmt.Println("cs-sleeper disabled")
}

// daemonRunning reports whether a cs-sleeper daemon is alive per its pid file.
func daemonRunning(pidFile string) bool {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	return processAlive(pid)
}

// setEnabledInFile sets the `enabled` key in the config file to yes/no,
// preserving all other lines and comments. A missing key is appended.
func setEnabledInFile(path string, enabled bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	val := "no"
	if enabled {
		val = "yes"
	}
	newLine := "enabled      = " + val
	lines := strings.Split(string(data), "\n")
	replaced := false
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "enabled") {
			lines[i] = newLine
			replaced = true
			break
		}
	}
	if !replaced {
		// Drop trailing empty lines (from the trailing newline), append the key,
		// and restore a single trailing newline.
		for len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, newLine, "")
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}
