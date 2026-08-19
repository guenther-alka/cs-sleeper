package sysio

import (
	"reflect"
	"testing"
)

func TestParseDiskutilInfo(t *testing.T) {
	// Real output captured from `diskutil info /` on macOS 12.7 (APFS root,
	// synthesized container disk3 backed by physical store disk2s2).
	apfs := `
   Device Identifier:         disk3s5s1
   Device Node:               /dev/disk3s5s1
   Whole:                     No
   Part of Whole:             disk3
   Volume Name:               monterey.12.196
   Mount Point:               /
   Type (Bundle):             apfs
   OS Can Be Installed:       No
   Booter Disk:               disk3s2
   Recovery Disk:             disk3s3
   Solid State:               Yes
   APFS Container:            disk3
   APFS Physical Store:       disk2s2
   Fusion Drive:              No
`

	hfs := `
   Device Identifier:         disk1s1
   Device Node:               /dev/disk1s1
   Whole:                     No
   Part of Whole:             disk1
   Volume Name:               MONTEREY
   Mount Point:               /
   Type (Bundle):             hfs
`

	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"apfs physical store", apfs, []string{"disk2"}},
		{"non-apfs device node", hfs, []string{"disk1"}},
		{"empty", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseDiskutilInfo(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("parseDiskutilInfo() = %v, want %v", got, c.want)
			}
		})
	}
}
