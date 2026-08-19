package sysio

import "strings"

// parseDiskutilInfo extracts the physical boot disk(s) from `diskutil info /`
// output. On APFS the root volume is a synthesized container (e.g. disk3)
// whose real backing device is the "APFS Physical Store" (e.g. disk2s2); that
// physical disk is the one we must never spin down. Non-APFS roots fall back
// to the "Device Node" line.
func parseDiskutilInfo(out string) []string {
	deviceNode := ""
	var physical []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "APFS Physical Store:"):
			for _, tok := range strings.Fields(strings.TrimPrefix(line, "APFS Physical Store:")) {
				if n := Normalize(tok); n != "" {
					physical = append(physical, n)
				}
			}
		case strings.HasPrefix(line, "Device Node:"):
			if deviceNode == "" {
				deviceNode = strings.TrimSpace(strings.TrimPrefix(line, "Device Node:"))
			}
		}
	}
	if len(physical) > 0 {
		return physical
	}
	if n := Normalize(deviceNode); n != "" {
		return []string{n}
	}
	return nil
}
