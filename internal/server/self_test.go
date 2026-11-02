package server

import (
	"net"
	"testing"

	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

func cidr(t *testing.T, s string) *net.IPNet {
	t.Helper()
	ip, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	n.IP = ip // keep the interface address, as net.Interface.Addrs does
	return n
}

func selfOf(cl []router.Client) []string {
	var out []string
	for _, c := range cl {
		if c.Self {
			out = append(out, c.IP)
		}
	}
	return out
}

func TestMarkSelfLoopback(t *testing.T) {
	local := localNets{nets: []*net.IPNet{cidr(t, "192.168.8.235/24")}, macs: map[string]bool{"2a:dc:9e:e2:f3:0a": true}}
	mk := func() []router.Client {
		return []router.Client{
			{IP: "192.168.8.100", MAC: "7a:b7:9c:a3:86:08", Online: true},
			{IP: "192.168.8.235", MAC: "00:00:5e:00:53:01", Online: true},  // same IP as this computer
			{IP: "192.168.8.50", MAC: "2A:DC:9E:E2:F3:0A", Online: true},   // same MAC (any letter case), another IP
			{IP: "192.168.8.235", MAC: "00:00:5e:00:53:02", Online: false}, // offline entries are never "this device"
		}
	}
	cl := mk()
	markSelf(cl, "127.0.0.1:5000", "192.168.8.1", local)
	if got := selfOf(cl); len(got) != 2 || got[0] != "192.168.8.235" || got[1] != "192.168.8.50" {
		t.Errorf("self = %v", got)
	}
	// the other router is on a different network: an IP that merely coincides must not be flagged
	cl = mk()
	markSelf(cl, "[::1]:5000", "192.168.18.1", local)
	if got := selfOf(cl); len(got) != 1 || got[0] != "192.168.8.50" { // only the MAC match counts
		t.Errorf("self on another network = %v", got)
	}
	// a router given by host name: no subnet check possible, the IP match is accepted
	cl = mk()
	markSelf(cl, "127.0.0.1:5000", "router.lan", local)
	if len(selfOf(cl)) != 2 {
		t.Errorf("host name router: %v", selfOf(cl))
	}
}

func TestMarkSelfRemoteViewer(t *testing.T) {
	cl := []router.Client{{IP: "192.168.8.100", Online: true}, {IP: "192.168.8.235", Online: true}}
	markSelf(cl, "192.168.8.100:51234", "192.168.8.1", localNets{}) // opened from a phone with -lan
	if got := selfOf(cl); len(got) != 1 || got[0] != "192.168.8.100" {
		t.Errorf("remote viewer: %v", got)
	}
}
