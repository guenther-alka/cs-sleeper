//go:build windows

package replcheck

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/xpath"
)

// cmdTimeout bounds the powershell/WMI query.
const cmdTimeout = 15 * time.Second

// cacheTTL bounds how long a cached result is reused. OpenZFS on Windows
// ships zfs.exe with the standard send/receive subcommands, and napp-it CS's
// own replication feature (job-replicate.pl) can run zfs send/receive
// locally on a Windows member -- so this is a real case here, not merely a
// theoretical one, and Windows needed a real Check() (previously this always
// reported inactive). Unlike Unix there is no pgrep: the process list (with
// command lines, to tell "zfs send ..." apart from an unrelated "zfs list")
// has to come from WMI via a powershell.exe invocation, which is too
// expensive to spawn on every daemon tick (default interval: 5s -- that
// would be a new powershell.exe process roughly every 5 seconds, forever).
// The result is cached for cacheTTL and refreshed lazily; a fresh one-shot
// invocation (sleepnow/sleeppool/export-now) always starts as a new process
// with an empty cache, so it still always performs a real, uncached check --
// only the long-running daemon loop benefits from (and needs) the caching.
// This matters most in practice for backup-pool export (sleeppool --export/
// export-now, or a scheduled forced export): an active pool's own idle/
// verify-idle requirement already keeps it from sleeping while replication
// I/O is ongoing in most cases, but export is a directed action that does
// not wait for idle, so this guard is its main protection on Windows.
const cacheTTL = 30 * time.Second

func powershellPath() string {
	return xpath.Resolve("powershell",
		`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`)
}

var (
	mu       sync.Mutex
	cached   Activity
	cachedAt time.Time
)

// Check reports whether any zfs send/receive is running, per a WMI process
// query (Win32_Process) filtered by executable name and command line.
func Check() Activity {
	mu.Lock()
	if !cachedAt.IsZero() && time.Since(cachedAt) < cacheTTL {
		a := cached
		mu.Unlock()
		return a
	}
	mu.Unlock()

	a := query()

	mu.Lock()
	cached, cachedAt = a, time.Now()
	mu.Unlock()
	return a
}

func query() Activity {
	var a Activity
	// Win32_Process.Name is the bare executable filename regardless of
	// install directory, so this needs no absolute-path resolution (unlike
	// actually invoking zpool/zfs, where xpath.Resolve matters).
	script := "Get-WmiObject Win32_Process -Filter \"Name='zfs.exe'\" | " +
		"Where-Object { $_.CommandLine -match 'send|receive|recv' } | " +
		"Select-Object -ExpandProperty ProcessId"
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	out, err := exec.CommandContext(ctx, powershellPath(), "-NoProfile", "-Command", script).Output()
	cancel()
	if err != nil {
		// No zfs.exe process at all (WMI returns nothing, not an error) is
		// the common case; a real query failure (e.g. WMI unavailable) also
		// lands here and is treated the same as "nothing found" -- matching
		// the Unix pgrep path, where a pgrep exit-with-no-matches is not
		// distinguished from a pgrep execution failure either.
		return a
	}
	for _, line := range strings.Fields(string(out)) {
		if _, convErr := strconv.Atoi(strings.TrimSpace(line)); convErr == nil {
			a.Processes = append(a.Processes, line)
		}
	}
	a.Active = len(a.Processes) > 0
	return a
}
