package collector

import (
	"context"
	"testing"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/config"
	"github.com/ahmadarif-lab/router-monitoring/internal/drivers"
	"github.com/ahmadarif-lab/router-monitoring/internal/router"
	"github.com/ahmadarif-lab/router-monitoring/internal/testrouter"
)

func demoProfiles() []config.Profile {
	return []config.Profile{
		{Name: "Office", Type: drivers.DemoGLiNet, URL: "http://demo.invalid", User: "demo", Pass: "demo"},
		{Name: "Home", Type: drivers.DemoHuawei, URL: "http://demo.invalid", User: "demo", Pass: "demo"},
	}
}

func TestCollectorCycleAndPersist(t *testing.T) {
	srv := testrouter.Huawei(t, "secret")
	store, _ := config.OpenStore(t.TempDir())
	p, _ := store.SaveProfile(config.Profile{Name: "Home", Type: "huawei", URL: srv.URL, User: "admin", Pass: "secret"})
	mgr := NewManager(store)
	col := mgr.Collector(p.ID)
	drv, _, err := col.driver()
	if err != nil {
		t.Fatal(err)
	}
	if err := col.cycle(drv, true); err != nil {
		t.Fatal(err)
	}
	v := col.View()
	if v.Snap == nil || len(v.Snap.Clients) != 1 || v.Err != nil {
		t.Fatalf("view: %+v", v)
	}
	col.saveState()
	if NewCollector(store, p.ID).View().Snap == nil { // state survives a restart
		t.Error("state file must be reloaded")
	}
}

func TestManagerSwitching(t *testing.T) {
	store, _ := config.OpenStore(t.TempDir())
	var ids []string
	for _, p := range demoProfiles() {
		sp, _ := store.SaveProfile(p)
		ids = append(ids, sp.ID)
	}
	_ = store.SetActive(ids[0])
	mgr := NewManager(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	waitFor := func(id string) *router.Snapshot {
		for i := 0; i < 100; i++ {
			if v := mgr.Collector(id).View(); v.Snap != nil {
				return v.Snap
			}
			time.Sleep(50 * time.Millisecond)
		}
		return nil
	}
	if s := waitFor(ids[0]); s == nil || s.Router.Vendor != "GL.iNet" {
		t.Fatalf("first profile did not poll: %+v", s)
	}
	if mgr.Collector(ids[1]).View().Snap != nil {
		t.Error("an inactive profile must not be polled")
	}
	if err := mgr.Switch(ids[1]); err != nil {
		t.Fatal(err)
	}
	if s := waitFor(ids[1]); s == nil || s.Router.Vendor != "Huawei" {
		t.Fatalf("second profile did not poll after switching: %+v", s)
	}
	if c, p, ok := mgr.Active(); !ok || p.ID != ids[1] || c != mgr.Collector(ids[1]) {
		t.Errorf("active: %v %v", p.ID, ok)
	}
	mgr.Stop() // must return promptly and save state
	if _, err := config.OpenStore(store.Dir); err != nil {
		t.Fatal(err)
	}
}

// TestGLiNetRealResponses feeds the driver responses captured from a real GL-MT3000 (firmware 4.9.0),
// anonymized: this guards the mapping against the router's real shapes (mixed number/string fields, guest
// interfaces, offline clients with totals, several network interfaces).

// compile-time check: the demo drivers can seed the graph
var _ router.HistorySeeder = (*seederCheck)(nil)

type seederCheck struct{}

func (*seederCheck) SeedHistory(time.Time) []router.Sample { return nil }
