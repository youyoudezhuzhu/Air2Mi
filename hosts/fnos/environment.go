package main

import (
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"
)

func defaultBind() string {
	if candidates := bindCandidates(); len(candidates) > 0 {
		return candidates[0]
	}
	return "127.0.0.1"
}

// bindCandidates lists usable multicast-capable IPv4 interfaces. Route-probe
// defaults are unreliable on NAS hosts with management, Docker and overlay
// networks, so discovery verifies candidates by actual mDNS responses.
func bindCandidates() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	score := func(name string) int {
		n := strings.ToLower(name)
		for _, bad := range []string{"docker", "veth", "zt", "tailscale", "wireguard", "tun", "tap", "lo", "virbr", "br-"} {
			if strings.Contains(n, bad) {
				return -100
			}
		}
		for _, preferred := range []string{"enp", "eno", "ens", "eth", "wlan", "wl"} {
			if strings.HasPrefix(n, preferred) {
				return 100
			}
		}
		if strings.HasPrefix(n, "enx") {
			return 50
		}
		return 0
	}
	type candidate struct {
		ip    string
		score int
	}
	var all []candidate
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagMulticast == 0 || score(iface.Name) < 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, raw := range addresses {
			var ip net.IP
			switch a := raw.(type) {
			case *net.IPNet:
				ip = a.IP
			case *net.IPAddr:
				ip = a.IP
			}
			if ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			all = append(all, candidate{ip: ip.To4().String(), score: score(iface.Name)})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].score > all[j].score })
	seen := map[string]bool{}
	result := make([]string, 0, len(all))
	for _, c := range all {
		if !seen[c.ip] {
			result = append(result, c.ip)
			seen[c.ip] = true
		}
	}
	return result
}
func environment() map[string]any {
	glibc := "unknown"
	if out, e := exec.Command("getconf", "GNU_LIBC_VERSION").Output(); e == nil {
		glibc = strings.TrimSpace(string(out))
	}
	_, dbus := os.Stat("/run/dbus/system_bus_socket")
	_, avahi := os.Stat("/run/avahi-daemon/socket")
	return map[string]any{"platform": "linux", "architecture": "x64", "glibc": glibc, "dbus_socket": dbus == nil, "avahi_socket": avahi == nil, "requirements": "glibc >= 2.36; system D-Bus and Avahi; doctor validates packaged codec/crypto"}
}

func hostPath(current string) string {
	parts := []string{}
	for _, entry := range strings.Split(current, string(os.PathListSeparator)) {
		if strings.HasPrefix(entry, "/") {
			parts = append(parts, entry)
		}
	}
	for _, path := range []string{"/usr/local/sbin", "/usr/local/bin", "/usr/sbin", "/usr/bin", "/sbin", "/bin"} {
		found := false
		for _, existing := range parts {
			if existing == path {
				found = true
			}
		}
		if !found {
			parts = append(parts, path)
		}
	}
	return strings.Join(parts, string(os.PathListSeparator))
}
