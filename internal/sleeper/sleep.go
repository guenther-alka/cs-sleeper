package sleeper

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/sysio"
	"github.com/guenther-alka/cs-sleeper/internal/xpath"
)

const cmdTimeout = 30 * time.Second

// Sleep spins a device down immediately via smartctl standby,now.
func Sleep(device string) (string, error) {
	if !sysio.Valid(device) {
		return "", fmt.Errorf("invalid device name %q", device)
	}
	return run("smartctl", "-s", "standby,now", sysio.DevicePath(device))
}

// Wake spins a device up via smartctl -s on.
func Wake(device string) (string, error) {
	if !sysio.Valid(device) {
		return "", fmt.Errorf("invalid device name %q", device)
	}
	return run("smartctl", "-s", "on", sysio.DevicePath(device))
}

// SetStandbyTimer sets the drive-internal standby timer (minutes).
func SetStandbyTimer(device string, minutes int) (string, error) {
	if !sysio.Valid(device) {
		return "", fmt.Errorf("invalid device name %q", device)
	}
	return run("smartctl", "-s", "standby,"+strconv.Itoa(minutes), sysio.DevicePath(device))
}

func run(name string, args ...string) (string, error) {
	if name == "smartctl" {
		name = smartctlPath()
	}
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("timed out after %s", cmdTimeout)
	}
	return string(out), err
}

// smartctlPath resolves the smartctl executable. It prefers well-known
// absolute install locations over a PATH search -- cs-sleeper normally runs
// as root/Administrator, so resolving purely via $PATH would let a writable
// directory earlier in PATH shadow the real binary. The standard
// smartmontools macOS installer puts smartctl in /usr/local/sbin, which is
// not on the default PATH used by daemons, launchd and non-interactive SSH
// sessions -- hence it (and the other common locations) are checked before
// falling back to a plain PATH lookup.
func smartctlPath() string {
	return xpath.Resolve("smartctl",
		"/usr/local/sbin/smartctl",
		"/opt/local/sbin/smartctl",
		"/opt/homebrew/sbin/smartctl",
		"/usr/sbin/smartctl",
		"/sbin/smartctl",
		"/usr/bin/smartctl",
		`C:\Program Files\smartmontools\bin\smartctl.exe`,
		`C:\Program Files (x86)\smartmontools\bin\smartctl.exe`,
	)
}

// SleepAll spins a set of devices down, running at most `parallel` smartctl
// invocations concurrently (0 = unlimited). It returns one error per failed
// device.
func SleepAll(devices []string, parallel int) []error {
	return spinAll(devices, parallel, "standby,now")
}

// WakeAll spins a set of devices up, at most `parallel` at a time (0 = unlimited).
func WakeAll(devices []string, parallel int) []error {
	return spinAll(devices, parallel, "on")
}

func spinAll(devices []string, parallel int, sub string) []error {
	if parallel <= 0 {
		parallel = len(devices)
		if parallel < 1 {
			parallel = 1
		}
	}
	sem := make(chan struct{}, parallel)
	var (
		mu   sync.Mutex
		errs []error
		wg   sync.WaitGroup
	)
	for _, d := range devices {
		wg.Add(1)
		go func(dev string) {
			defer wg.Done()
			if !sysio.Valid(dev) {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: invalid device name", dev))
				mu.Unlock()
				return
			}
			sem <- struct{}{}
			defer func() { <-sem }()
			if _, err := run("smartctl", "-s", sub, sysio.DevicePath(dev)); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", dev, err))
				mu.Unlock()
			}
		}(d)
	}
	wg.Wait()
	return errs
}
