package sleeper

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
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
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("timed out after %s", cmdTimeout)
	}
	return string(out), err
}
