//go:build !illumos && !solaris

package zfs

import (
	"fmt"

	"github.com/guenther-alka/cs-sleeper/internal/sysio"
)

// Sync flushes pool's in-core dirty data to its primary storage (and not the
// ZIL) via `zpool sync`, so disks that are about to be spun down do not wake
// up again on the next ZFS transaction-group commit.
func Sync(pool string) (string, error) {
	if !sysio.Valid(pool) {
		return "", fmt.Errorf("invalid pool name %q", pool)
	}
	return run("sync", pool)
}
