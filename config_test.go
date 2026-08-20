package main

import "testing"

func TestMinWindowActive(t *testing.T) {
	same := MinWindow{Start: 7 * 60, End: 19 * 60} // 07:00-19:00
	for _, m := range []int{7 * 60, 12 * 60, 19*60 - 1} {
		if !same.Active(m) {
			t.Errorf("07:00-19:00 should be active at minute %d", m)
		}
	}
	for _, m := range []int{6*60 + 59, 19 * 60, 0} {
		if same.Active(m) {
			t.Errorf("07:00-19:00 should NOT be active at minute %d", m)
		}
	}

	cross := MinWindow{Start: 22 * 60, End: 6 * 60} // 22:00-06:00, crosses midnight
	for _, m := range []int{22 * 60, 23*60 + 59, 0, 5 * 60} {
		if !cross.Active(m) {
			t.Errorf("22:00-06:00 should be active at minute %d", m)
		}
	}
	for _, m := range []int{21*60 + 59, 6 * 60, 12 * 60} {
		if cross.Active(m) {
			t.Errorf("22:00-06:00 should NOT be active at minute %d", m)
		}
	}
}

func TestParseHHMMWindows(t *testing.T) {
	ws, err := parseHHMMWindows("07:00-19:00,22:30-23:45")
	if err != nil {
		t.Fatal(err)
	}
	want := []MinWindow{{7 * 60, 19 * 60}, {22*60 + 30, 23*60 + 45}}
	if len(ws) != len(want) || ws[0] != want[0] || ws[1] != want[1] {
		t.Fatalf("got %v, want %v", ws, want)
	}

	if _, err := parseHHMMWindows("7-19"); err == nil {
		t.Fatal("expected error for hour-only range (not HH:MM)")
	}
	if _, err := parseHHMMWindows("25:00-26:00"); err == nil {
		t.Fatal("expected error for out-of-range hour")
	}
	if _, err := parseHHMMWindows("07:60-19:00"); err == nil {
		t.Fatal("expected error for out-of-range minute")
	}

	// Round-trip through the formatter.
	if got := formatMinWindows(ws); got != "07:00-19:00,22:30-23:45" {
		t.Fatalf("formatMinWindows round-trip: got %q", got)
	}
}

func TestParsePoolWindows(t *testing.T) {
	pw, err := parsePoolWindows("tank:07:00-19:00,20:00-22:00;backup:22:00-06:00")
	if err != nil {
		t.Fatal(err)
	}
	if len(pw) != 2 {
		t.Fatalf("got %d pools, want 2", len(pw))
	}
	if len(pw["tank"]) != 2 {
		t.Fatalf("tank: got %d windows, want 2", len(pw["tank"]))
	}
	if len(pw["backup"]) != 1 || pw["backup"][0] != (MinWindow{22 * 60, 6 * 60}) {
		t.Fatalf("backup: got %v", pw["backup"])
	}

	if got, err := parsePoolWindows(""); err != nil || got != nil {
		t.Fatalf("empty input: got %v, %v", got, err)
	}
	if _, err := parsePoolWindows("noPoolNameHere"); err == nil {
		t.Fatal("expected error for entry missing pool:windows separator")
	}

	// Round-trip through the formatter (pool names sorted for stability).
	if got := formatPoolWindow(pw); got != "backup:22:00-06:00;tank:07:00-19:00,20:00-22:00" {
		t.Fatalf("formatPoolWindow round-trip: got %q", got)
	}
}

func TestValidateConfigOverlap(t *testing.T) {
	c := &Config{
		Pools:           []string{"tank"},
		ExportPools:     []string{"tank"},
		ExportTimetable: []MinWindow{{7 * 60, 19 * 60}, {15 * 60, 20 * 60}}, // overlaps 15:00-19:00
	}
	if err := validateConfig(c); err == nil {
		t.Fatal("expected overlap error")
	}

	c.ExportTimetable = []MinWindow{{7 * 60, 12 * 60}, {12 * 60, 20 * 60}} // back-to-back, not overlapping
	if err := validateConfig(c); err != nil {
		t.Fatalf("back-to-back windows should not be rejected: %v", err)
	}

	// Two midnight-crossing windows that overlap in their wrapped segment.
	c.ExportTimetable = []MinWindow{{22 * 60, 6 * 60}, {2 * 60, 10 * 60}}
	if err := validateConfig(c); err == nil {
		t.Fatal("expected overlap error across a midnight-crossing window")
	}
}

func TestValidateConfigUnknownPool(t *testing.T) {
	c := &Config{
		Pools:           []string{"tank"},
		ExportPools:     []string{"backup"}, // not in Pools
		ExportTimetable: []MinWindow{{7 * 60, 19 * 60}},
	}
	if err := validateConfig(c); err == nil {
		t.Fatal("expected error: export-pools references a pool not in pools")
	}

	c2 := &Config{
		Pools:      []string{"tank"},
		PoolWindow: map[string][]MinWindow{"other": {{7 * 60, 19 * 60}}}, // not in Pools
	}
	if err := validateConfig(c2); err == nil {
		t.Fatal("expected error: pool-window references a pool not in pools")
	}
}

func TestParseConfigExportAndWindowKeys(t *testing.T) {
	text := "pools = tank,backup\n" +
		"export-pools = backup\n" +
		"export-timetable = 07:00-19:00\n" +
		"pool-window = tank:06:00-22:00\n"
	c := defaultConfig()
	if err := parseConfig(text, c); err != nil {
		t.Fatal(err)
	}
	if len(c.ExportPools) != 1 || c.ExportPools[0] != "backup" {
		t.Fatalf("ExportPools: got %v", c.ExportPools)
	}
	if len(c.ExportTimetable) != 1 || c.ExportTimetable[0] != (MinWindow{7 * 60, 19 * 60}) {
		t.Fatalf("ExportTimetable: got %v", c.ExportTimetable)
	}
	if len(c.PoolWindow["tank"]) != 1 || c.PoolWindow["tank"][0] != (MinWindow{6 * 60, 22 * 60}) {
		t.Fatalf("PoolWindow[tank]: got %v", c.PoolWindow["tank"])
	}

	// export-pools referencing a pool absent from `pools` must fail
	// validation (exercises the loadConfig-level post-parse check).
	bad := "pools = tank\nexport-pools = backup\nexport-timetable = 07:00-19:00\n"
	if err := parseConfig(bad, defaultConfig()); err == nil {
		t.Fatal("expected validation error for export-pools referencing an unknown pool")
	}
}
