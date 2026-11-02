package restart

import (
	"os"
	"testing"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/collector"
	"github.com/ahmadarif-lab/router-monitoring/internal/config"
	"github.com/ahmadarif-lab/router-monitoring/internal/drivers"
)

func TestScheduleDue(t *testing.T) {
	loc := time.Local
	at := func(h, m int) time.Time { return time.Date(2026, 11, 2, h, m, 0, 0, loc) }
	sc := config.Schedule{Enabled: true, Time: "03:00"}
	if scheduleDue(sc, at(2, 59), 0, time.Time{}) {
		t.Error("before time")
	}
	if !scheduleDue(sc, at(3, 0), 0, time.Time{}) || !scheduleDue(sc, at(3, 20), 0, time.Time{}) {
		t.Error("inside window must run (also when late after sleep)")
	}
	if scheduleDue(sc, at(3, 45), 0, time.Time{}) {
		t.Error("after the window must not run")
	}
	sc.LastRun = "2026-11-02"
	if scheduleDue(sc, at(3, 5), 0, time.Time{}) {
		t.Error("already ran today")
	}
	if !scheduleDue(sc, at(3, 5), 1, at(3, 2)) || scheduleDue(sc, at(3, 5), 3, at(3, 2)) {
		t.Error("retry logic")
	}
	sc.Enabled = false
	if scheduleDue(sc, at(3, 5), 1, at(3, 2)) {
		t.Error("disabled")
	}
}

func TestSlotUsed(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 11, 2, h, m, 0, 0, time.Local) }
	if got := SlotUsed("03:00", at(3, 10)); got != "2026-11-02" {
		t.Errorf("slot already passed must count as used, got %q", got)
	}
	if got := SlotUsed("03:00", at(1, 0)); got != "" {
		t.Errorf("slot still ahead must stay open, got %q", got)
	}
	// enabling at 03:10 for 03:00 must not fire right away
	sc := config.Schedule{Enabled: true, Time: "03:00", LastRun: SlotUsed("03:00", at(3, 10))}
	if scheduleDue(sc, at(3, 10), 0, time.Time{}) {
		t.Error("must not restart immediately after enabling")
	}
}

func TestRestartDryRun(t *testing.T) {
	store, _ := config.OpenStore(t.TempDir())
	p, _ := store.SaveProfile(config.Profile{Name: "Home", Type: drivers.DemoHuawei, URL: "http://demo.invalid", User: "demo", Pass: "demo"})
	mgr := collector.NewManager(store)
	rst := NewRestarter(store, mgr)
	if err := rst.Now(p.ID, true, "test"); err != nil {
		t.Errorf("dry restart: %v", err)
	}
	if _, err := os.Stat(store.Path("restart.log")); err != nil {
		t.Errorf("restart.log must be written: %v", err)
	}
	if err := rst.Now("nope", true, "test"); err == nil {
		t.Error("restart of an unknown profile must fail")
	}
}
