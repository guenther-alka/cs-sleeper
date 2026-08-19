//go:build !windows

package replcheck

import (
	"os/exec"
	"strings"
)

// Check reports whether any zfs send/receive is running on this host.
func Check() Activity {
	var a Activity
	for _, pat := range []string{"[z]fs send", "[z]fs receive", "[z]fs recv"} {
		out, err := exec.Command("pgrep", "-f", pat).Output()
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
