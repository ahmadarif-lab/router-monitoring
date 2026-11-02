package collector

import (
	"math"

	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

type Counter struct {
	T    float64 `json:"t"`
	Up   uint64  `json:"up"`
	Down uint64  `json:"down"`
}

// Traffic builds the Mbps history, either from cumulative byte counters or from a reported speed.
type Traffic struct {
	Hist []router.Sample
	Last *Counter
}

func (t *Traffic) trim(now float64) {
	cut := now - historySecs
	i := 0
	for i < len(t.Hist) && t.Hist[i][0] < cut {
		i++
	}
	t.Hist = t.Hist[i:]
}

func (t *Traffic) add(now, upMbps, downMbps float64) {
	if upMbps < 3000 && downMbps < 3000 { // drop bogus spikes
		t.Hist = append(t.Hist, router.Sample{math.Round(now*10) / 10, math.Round(upMbps*100) / 100, math.Round(downMbps*100) / 100})
	}
}

// Record turns two successive counter readings into a rate.
func (t *Traffic) Record(now float64, c router.Counters) {
	if t.Last != nil && now > t.Last.T {
		dt := now - t.Last.T
		du, ok1 := delta(c.Up, t.Last.Up)
		dd, ok2 := delta(c.Down, t.Last.Down)
		if dt <= maxSampleGap && ok1 && ok2 {
			t.add(now, float64(du)*8/dt/1e6, float64(dd)*8/dt/1e6)
		}
	}
	t.Last = &Counter{T: now, Up: c.Up, Down: c.Down}
	t.trim(now)
}

// RecordSpeed stores a speed the router reported itself (bytes per second).
func (t *Traffic) RecordSpeed(now float64, s router.Speed) {
	t.add(now, s.Up*8/1e6, s.Down*8/1e6)
	t.trim(now)
}

// delta returns the byte difference between two 32-bit counter readings, handling wrap-around.
// ok=false means the counter was reset (router reboot), so the sample must be skipped.
func delta(cur, last uint64) (uint64, bool) {
	if cur >= last {
		return cur - last, true
	}
	if last > 3<<30 {
		return cur + 1<<32 - last, true
	}
	return 0, false
}
