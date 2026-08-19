package sysio

import (
	"reflect"
	"testing"
)

func TestParseDarwinIostat(t *testing.T) {
	in := `      disk0       disk1
    KB/t  tps  MB/s     KB/t  tps  MB/s
    16.00  2  0.03      4.00  0  0.00
      disk0       disk1
    KB/t  tps  MB/s     KB/t  tps  MB/s
     8.00  0  0.00      4.00  1  0.01
`
	want := []Counter{
		{Device: "disk0", Active: false},
		{Device: "disk1", Active: true},
	}
	if got := parseDarwinIostat(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseDarwinIostatSingleDisk(t *testing.T) {
	in := `    disk0
    KB/t tps  MB/s
    4.00  0  0.00
    disk0
    KB/t tps  MB/s
    6.00  3  0.02
`
	want := []Counter{{Device: "disk0", Active: true}}
	if got := parseDarwinIostat(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseDarwinIostatIgnoresNonDiskSections(t *testing.T) {
	in := `cpu
us sy id
 5  2 93
      disk0
    KB/t tps  MB/s
    4.00  1  0.00
`
	want := []Counter{{Device: "disk0", Active: true}}
	if got := parseDarwinIostat(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
