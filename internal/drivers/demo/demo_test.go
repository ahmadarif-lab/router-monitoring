package demo

import (
	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/router"
	"strings"
	"testing"
	"time"
)

func TestDemoDrivers(t *testing.T) {
	gl, hw := newDemo(true), newDemo(false)
	g, err := gl.Poll(false)
	if err != nil || g.Snap.System == nil || g.Speed == nil || g.Snap.Clients[0].Rate == nil || g.Snap.Router.Vendor != "GL.iNet" {
		t.Fatalf("glinet demo: %v %+v", err, g)
	}
	h, err := hw.Poll(false)
	if err != nil || h.Snap.System != nil || h.Counters == nil || h.Snap.Clients[0].Signal == nil || h.Snap.Router.Vendor != "Huawei" {
		t.Fatalf("huawei demo: %v %+v", err, h)
	}
	for _, c := range append(g.Snap.Clients, h.Snap.Clients...) {
		if !strings.HasPrefix(c.MAC, "00:00:5e:00:53:") {
			t.Errorf("demo MAC outside the documentation range: %s", c.MAC)
		}
	}
	if n := len(gl.SeedHistory(time.Now())); n < 300 {
		t.Errorf("seed history too short: %d", n)
	}
	for name, snap := range map[string]*router.Snapshot{"glinet": g.Snap, "huawei": h.Snap} {
		n := 0
		for _, c := range snap.Clients {
			if c.Self {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s demo must show exactly one \"this device\", got %d", name, n)
		}
	}
	_ = hw.Reboot(false)
	if hw.Alive() {
		t.Error("demo router must be down right after a reboot")
	}
	if _, err := hw.Poll(true); apperr.From(err).Code != "unreachable" {
		t.Errorf("polling a rebooting demo router: %v", err)
	}
}
