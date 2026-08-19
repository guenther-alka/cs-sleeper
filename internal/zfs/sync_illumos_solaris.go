//go:build illumos || solaris

package zfs

import "os/exec"

// Sync flushes all in-core dirty data to stable storage before disks are put
// to standby. illumos/Solaris native ZFS has no `zpool sync` subcommand, so we
// fall back to the POSIX `sync` command, which commits every filesystem. The
// pool argument is ignored on these platforms.
func Sync(pool string) (string, error) {
	out, err := exec.Command("sync").CombinedOutput()
	return string(out), err
}
