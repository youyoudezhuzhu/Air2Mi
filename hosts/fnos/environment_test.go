package main

import (
	"net"
	"testing"
)

func TestBindCandidatesAreUniqueUsableIPv4(t *testing.T) {
	seen := map[string]bool{}
	for _, raw := range bindCandidates() {
		ip := net.ParseIP(raw)
		if ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			t.Fatalf("invalid bind candidate %q", raw)
		}
		if seen[raw] {
			t.Fatalf("duplicate candidate %q", raw)
		}
		seen[raw] = true
	}
}
