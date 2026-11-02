package collector

import (
	"encoding/json"
	"log"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

type persisted struct {
	Snap    *router.Snapshot `json:"snap"`
	SnapAt  float64          `json:"ts"`
	History []router.Sample  `json:"history"`
	Counter *Counter         `json:"counter"`
}

func (c *Collector) stateFile() string { return "state-" + c.id + ".json" }

func (c *Collector) saveState() {
	c.mu.RLock()
	p := persisted{Snap: c.snap, SnapAt: float64(c.snapAt.UnixNano()) / 1e9, History: c.traffic.Hist, Counter: c.traffic.Last}
	b, err := json.Marshal(p)
	c.mu.RUnlock()
	if err != nil {
		return
	}
	if err := c.store.WriteFile(c.stateFile(), b, 0o600); err != nil {
		log.Printf("could not save state: %v", err)
	}
}

func (c *Collector) loadState() {
	b, err := c.store.ReadFile(c.stateFile())
	if err != nil {
		return
	}
	var p persisted
	if json.Unmarshal(b, &p) != nil {
		return
	}
	cut := float64(time.Now().Unix() - historySecs)
	for _, s := range p.History {
		if s[0] >= cut {
			c.traffic.Hist = append(c.traffic.Hist, s)
		}
	}
	c.traffic.Last = p.Counter
	c.snap = p.Snap
	if p.Snap != nil && p.SnapAt > 0 {
		c.snapAt = time.Unix(int64(p.SnapAt), 0)
	}
}
