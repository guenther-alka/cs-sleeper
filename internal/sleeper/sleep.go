package sleeper

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/sysio"
)

const cmdTimeout = 30 * time.Second

// Sleep spins a device down immediately via smartctl standby,now.
func Sleep(device string) (string, error) {
	return run("smartctl", "-s", "standby,now", sysio.DevicePath(device))
}

// Wake spins a device up via smartctl -s on.
func Wake(device string) (string, error) {
	return run("smartctl", "-s", "on", sysio.DevicePath(device))
}

// SetStandbyTimer sets the drive-internal standby timer (minutes).
func SetStandbyTimer(device string, minutes int) (string, error) {
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

// smartctlPath resolves the smartctl executable. It prefers the PATH lookup but
// falls back to common absolute locations: the standard smartmontools macOS
// installer puts smartctl in /usr/local/sbin, which is not on the default PATH
// used by daemons, launchd and non-interactive SSH sessions.
func smartctlPath() string {
	if p, err := exec.LookPath("smartctl"); err == nil {
		return p
	}
	for _, p := range []string{
		"/usr/local/sbin/smartctl",
		"/opt/local/sbin/smartctl",
		"/opt/homebrew/sbin/smartctl",
		"/usr/sbin/smartctl",
		"/sbin/smartctl",
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return "smartctl"
}

// SleepAll spins a set of devices down, running at most `parallel` smartctl
// invocations concurrently. It returns one error per failed device.
func SleepAll(devices []string, parallel int) []error {
	return spinAll(devices, parallel, "standby,now")
}

// WakeAll spins a set of devices up, at most `parallel` at a time.
func WakeAll(devices []string, parallel int) []error {
	return spinAll(devices, parallel, "on")
}

func spinAll(devices []string, parallel int, sub string) []error {
	if parallel <= 0 {
		parallel = 1
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
