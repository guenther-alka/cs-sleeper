// Package replcheck detects whether ZFS replication (zfs send/receive) is
// currently running, so the sleeper can refuse to sleep disks or export
// pools mid-transfer.
package replcheck

// Activity reports running replication.
type Activity struct {
	Active bool
	// Processes holds best-effort process identifiers (PIDs) of running
	// zfs send/receive invocations.
	Processes []string
}
