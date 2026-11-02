package collector

import (
	"testing"

	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

func TestDelta(t *testing.T) {
	if d, ok := delta(150, 100); !ok || d != 50 {
		t.Errorf("normal: %d %v", d, ok)
	}
	if d, ok := delta(10, 4294967290); !ok || d != 16 { // 32-bit wrap
		t.Errorf("wrap: %d %v", d, ok)
	}
	if _, ok := delta(10, 5000); ok { // counter reset (router reboot)
		t.Errorf("reset should be skipped")
	}
}

func TestTrafficRecord(t *testing.T) {
	var tr Traffic
	tr.Record(1000, router.Counters{Up: 0, Down: 0})
	tr.Record(1010, router.Counters{Up: 12_500_000, Down: 125_000_000}) // 10s -> 10 Mbps up, 100 Mbps down
	if len(tr.Hist) != 1 || tr.Hist[0][1] != 10 || tr.Hist[0][2] != 100 {
		t.Fatalf("rate wrong: %v", tr.Hist)
	}
	tr.Record(1100, router.Counters{Up: 12_500_001, Down: 125_000_001}) // 90s gap: not averaged
	if len(tr.Hist) != 1 {
		t.Errorf("long gap must be skipped: %v", tr.Hist)
	}
	tr.Record(1110, router.Counters{Up: 1, Down: 1}) // reset
	if len(tr.Hist) != 1 {
		t.Errorf("reset must be skipped: %v", tr.Hist)
	}
	tr.Record(5000, router.Counters{Up: 1, Down: 1})
	tr.Record(5001, router.Counters{Up: 1, Down: 1})
	if tr.Hist[0][0] < 5000-historySecs {
		t.Errorf("old samples must be trimmed: %v", tr.Hist)
	}
}

func TestRecordSpeed(t *testing.T) {
	var tr Traffic
	tr.RecordSpeed(1000, router.Speed{Up: 12500, Down: 125000}) // bytes/s: 0.1 Mbps up, 1 Mbps down
	if len(tr.Hist) != 1 || tr.Hist[0][1] != 0.1 || tr.Hist[0][2] != 1 {
		t.Errorf("speed -> Mbps: %v", tr.Hist)
	}
	tr.RecordSpeed(1000+historySecs+10, router.Speed{})
	if len(tr.Hist) != 1 {
		t.Errorf("old samples must be trimmed: %v", tr.Hist)
	}
}
