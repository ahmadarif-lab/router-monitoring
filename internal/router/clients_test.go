package router

import (
	"testing"
)

func TestVendorHint(t *testing.T) {
	for _, c := range []struct{ host, vclass, mac, want string }{
		{"x", "MSFT 5.0", "00:11:22:33:44:55", "Windows"},
		{"x", "android-dhcp-13", "00:11:22:33:44:55", "Android 13"},
		{"Ahmads-MacBook-Pro", "", "00:11:22:33:44:55", "Apple"},
		{"lwip0", "", "00:11:22:33:44:55", "IoT / ESP"},
		{"x", "", "56:4f:3b:bd:a3:69", TypeRandomMAC},
		{"x", "", "68:b9:c2:6d:bf:4e", ""},
	} {
		if got := VendorHint(c.host, c.vclass, c.mac); got != c.want {
			t.Errorf("VendorHint(%q,%q,%q)=%q want %q", c.host, c.vclass, c.mac, got, c.want)
		}
	}
}

func TestSortClients(t *testing.T) {
	cl := []Client{{IP: "192.168.1.20"}, {IP: "192.168.1.3", Online: true}, {IP: "192.168.1.100", Online: true}, {IP: ""}}
	SortClients(cl)
	got := []string{cl[0].IP, cl[1].IP, cl[2].IP, cl[3].IP}
	want := []string{"192.168.1.3", "192.168.1.100", "192.168.1.20", ""} // online first, numeric IP order, no IP last
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
