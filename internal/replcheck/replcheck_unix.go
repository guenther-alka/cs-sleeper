//go:build !windows

package replcheck

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/xpath"
)

const cmdTimeout = 10 * time.Second

func pgrepPath() string {
	return xpath.Resolve("pgrep", "/usr/bin/pgrep", "/bin/pgrep", "/usr/local/bin/pgrep")
}

// Check reports whether any zfs send/receive is running on this host.
func Check() Activity {
	var a Activity
	for _, pat := range []string{"[z]fs send", "[z]fs receive", "[z]fs recv"} {
		ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
		out, err := exec.CommandContext(ctx, pgrepPath(), "-f", pat).Output()
		cancel()
		if err != nil {
			continue
		}
		for _, p := range strings.Fields(string(out)) {
			a.Processes = append(a.Processes, p)
		}
	}
	a.Active = len(a.Processes) > 0
	return a
}
