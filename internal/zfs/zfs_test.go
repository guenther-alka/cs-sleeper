package zfs

import (
	"reflect"
	"testing"
)

func TestParseZpoolStatusClassification(t *testing.T) {
	in := `  pool: tank
 state: ONLINE
config:

	NAME        STATE     READ WRITE CKSUM
	tank        ONLINE       0     0     0
	  mirror-0  ONLINE       0     0     0
	    /dev/sda  ONLINE       0     0     0
	    /dev/sdb  ONLINE       0     0     0
	logs
	  /dev/da4  ONLINE       0     0     0
	cache
	  /dev/da5  ONLINE       0     0     0
	spares
	  /dev/sdc  AVAIL
`
	wantData := []string{"sda", "sdb", "sdc"}
	wantFlash := []string{"da4", "da5"}
	data, flash := parseZpoolStatus(in)
	if !reflect.DeepEqual(data, wantData) {
		t.Fatalf("data: got %v, want %v", data, wantData)
	}
	if !reflect.DeepEqual(flash, wantFlash) {
		t.Fatalf("flash: got %v, want %v", flash, wantFlash)
	}
}

func TestParseZpoolStatusNoFlash(t *testing.T) {
	in := `  pool: tank
 state: ONLINE
config:

	NAME        STATE     READ WRITE CKSUM
	tank        ONLINE       0     0     0
	  raidz1-0  ONLINE       0     0     0
	    /dev/da0  ONLINE       0     0     0
	    /dev/da1  ONLINE       0     0     0
`
	data, flash := parseZpoolStatus(in)
	if want := []string{"da0", "da1"}; !reflect.DeepEqual(data, want) {
		t.Fatalf("data: got %v, want %v", data, want)
	}
	if len(flash) != 0 {
		t.Fatalf("flash: got %v, want empty", flash)
	}
}

// TestParseZpoolStatusIllumosShortNames is the regression test for the
// false-"sleeping"/empty-disk-list bug root-caused live on illumos: `zpool
// status -P <pool>` there returns bare short logical names (no "/" prefix
// at all), which the old fields[0]-contains-"/" heuristic silently treated
// as non-device structural lines, so DisksOfPool returned zero disks. This
// is the real output captured from `zpool status -P b1` on an OmniOS host.
func TestParseZpoolStatusIllumosShortNames(t *testing.T) {
	in := ` pool: b1
 state: ONLINE
  scan: scrub repaired 0 in 0 days 05:19:08 with 0 errors on Sun Jan  5 18:05:35 2025
config:

	NAME         STATE     READ WRITE CKSUM
	b1           ONLINE       0     0     0
	  raidz1-0   ONLINE       0     0     0
	    c8d0s0   ONLINE       0     0     0
	    c9d0s0   ONLINE       0     0     0
	    c10d0s0  ONLINE       0     0     0

errors: No known data errors
`
	data, flash := parseZpoolStatus(in)
	want := []string{"c8d0", "c9d0", "c10d0"}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("data: got %v, want %v", data, want)
	}
	if len(flash) != 0 {
		t.Fatalf("flash: got %v, want empty", flash)
	}
}

// TestParseZpoolStatusIllumosWWNTarget covers a hex/WWN-style target id
// (c1t00253859019E709Dd0), also captured live from the same host, to make
// sure illumosDeviceRe isn't limited to single-digit target ids.
func TestParseZpoolStatusIllumosWWNTarget(t *testing.T) {
	in := `config:

	NAME                       STATE     READ WRITE CKSUM
	b2                         ONLINE       0     0     0
	  c1t00253859019E709Dd0s0  ONLINE       0     0     0
`
	data, _ := parseZpoolStatus(in)
	want := []string{"c1t00253859019e709dd0"}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("data: got %v, want %v", data, want)
	}
}
