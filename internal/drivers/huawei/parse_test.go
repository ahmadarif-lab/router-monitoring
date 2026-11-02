package huawei

import (
	"strings"
	"testing"

	"github.com/ahmadarif-lab/router-monitoring/internal/testrouter"
)

func TestUnesc(t *testing.T) {
	cases := map[string]string{
		`a\x3ab`:       "a:b",
		`\xe2\x9c\x93`: "✓",    // valid UTF-8
		`caf\xe9`:      "café", // invalid UTF-8 -> Latin-1
		"plain":        "plain",
	}
	for in, want := range cases {
		if got := unesc(in); got != want {
			t.Errorf("unesc(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseClients(t *testing.T) {
	dev := strings.Join([]string{
		testrouter.HuaweiDevice("192.168.18.20", "56:4f:3b:bd:a3:69", "SSID5", "android-dhcp-14", "Online", "WIFI", "Pixel"),
		testrouter.HuaweiDevice("192.168.18.3", "bc:07:1d:9f:49:bb", "LAN1", "MSFT 5.0", "Offline", "ETH", "PC"),
		testrouter.HuaweiDevice("192.168.18.10", "68:5e:dd:6f:fb:d9", "SSID1", "", "Online", "WIFI", "Ahmads-Air"),
	}, ";\n") + `;new stWifiWorkingMode("x","11ac","x","56\x3a4f\x3a3b\x3abd\x3aa3\x3a69")`
	assoc := `new stAssociatedDevice("0","56:4F:3B:BD:A3:69","120","866","780","-61","-95","34","x")`
	ssids := map[string][2]string{"1": {"HOME", "2.4GHz"}, "5": {"HOME_5G", "5GHz"}}

	got := parseClients(dev, ssids, assoc)
	if len(got) != 3 {
		t.Fatalf("want 3 clients, got %d", len(got))
	}
	if got[0].IP != "192.168.18.10" || got[1].IP != "192.168.18.20" || got[2].Online { // online first, by IP; offline last
		t.Errorf("sort order wrong: %+v", got)
	}
	p := got[1]
	if p.Conn != "WiFi 5 GHz" || p.Band != "5" || p.SSID != "HOME_5G" || p.Mode != "11ac" || p.Type != "Android 14" {
		t.Errorf("wifi client parsed wrong: %+v", p)
	}
	if p.Signal == nil || p.Signal.RSSI == nil || *p.Signal.RSSI != -61 || *p.Signal.RX != 866 {
		t.Errorf("signal parsed wrong: %+v", p.Signal)
	}
	if got[0].Band != "2.4" || got[2].Conn != "LAN 1" || got[2].Band != "lan" || got[2].Type != "Windows" {
		t.Errorf("other clients wrong: %+v %+v", got[0], got[2])
	}
	if got[2].Signal != nil {
		t.Errorf("offline client must have no signal")
	}
}

func TestParseWAN(t *testing.T) {
	ppp := `new WanPPP("a","b","c","d","Connected","f","g","ppp_1","i","j","k","l","m","10.1.2.3","10.1.2.1","p","q","8.8.8.8")`
	stats := `var WanEthPPStats = new Array(new WaninfoStats("x","1234","5678","y"),null);`
	w, c := parseWAN(ppp + "\n" + stats)
	if w.Online == nil || !*w.Online || w.IP != "10.1.2.3" || w.GW != "10.1.2.1" || w.DNS != "8.8.8.8" {
		t.Errorf("wan: %+v", w)
	}
	if c == nil || c.Up != 1234 || c.Down != 5678 {
		t.Errorf("counters: %+v", c)
	}
}
