//go:build !illumos && !solaris

package zfs

// Sync flushes pool's in-core dirty data to its primary storage (and not the
// ZIL) via `zpool sync`, so disks that are about to be spun down do not wake
// up again on the next ZFS transaction-group commit.
func Sync(pool string) (string, error) {
	return run("sync", pool)
}
