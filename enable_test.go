package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetEnabledInFile(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "cfg")
	if err := os.WriteFile(path, []byte("# comment\nenabled = no\nwait = 600\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setEnabledInFile(path, true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "# comment\nenabled      = yes\nwait = 600\n" {
		t.Fatalf("got %q", string(got))
	}

	// Missing key is appended.
	path2 := filepath.Join(dir, "cfg2")
	if err := os.WriteFile(path2, []byte("wait = 600\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setEnabledInFile(path2, false); err != nil {
		t.Fatal(err)
	}
	got2, _ := os.ReadFile(path2)
	if string(got2) != "wait = 600\nenabled      = no\n" {
		t.Fatalf("got %q", string(got2))
	}
}
