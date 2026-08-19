//go:build windows

package replcheck

// Check reports whether any zfs send/receive is running. On Windows there is
// no local zfs send/receive on the sleeper host (OpenZFS Windows is a storage
// target, not a replication sender/receiver), so this always reports inactive.
func Check() Activity { return Activity{} }
