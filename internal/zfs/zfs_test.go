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
