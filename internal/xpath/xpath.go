// Package xpath resolves external tool binaries to absolute, known-good
// paths before falling back to a PATH search.
//
// cs-sleeper normally runs with elevated privileges (root, or Administrator
// on Windows) because smartctl spin-down and zpool import/export require it.
// Resolving helper binaries (zpool, smartctl, qm, iostat, ...) purely via
// $PATH in that context is risky: a writable directory listed earlier in
// $PATH than the real binary's location could be used to plant a malicious
// "zpool" or "smartctl" and get code execution at the daemon's privilege
// level. Preferring fixed, well-known install locations closes most of that
// window; PATH is only consulted as a last resort, for hosts where the tool
// lives somewhere non-standard.
package xpath

import "os"
import "os/exec"

// Resolve returns the first existing regular file among candidates (checked
// in order). If none exist, it falls back to an exec.LookPath search for
// name, and finally returns name unresolved so the caller still gets a
// normal "executable file not found" error from exec.Command rather than an
// empty path.
func Resolve(name string, candidates ...string) string {
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}
