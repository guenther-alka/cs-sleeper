//go:build illumos || solaris

package zfs

import (
	"context"
	"os/exec"

	"github.com/guenther-alka/cs-sleeper/internal/xpath"
)

// Sync flushes all in-core dirty data to stable storage before disks are put
// to standby. illumos/Solaris native ZFS has no `zpool sync` subcommand, so we
// fall back to the POSIX `sync` command, which commits every filesystem. The
// pool argument is ignored on these platforms.
func Sync(pool string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	path := xpath.Resolve("sync", "/usr/bin/sync", "/bin/sync")
	out, err := exec.CommandContext(ctx, path).CombinedOutput()
	return string(out), err
}
