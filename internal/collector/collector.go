// Package collector is the ONLY code that talks to a router. A Collector polls one router in the background
// and keeps the latest snapshot and the traffic history in memory (persisted to state-<profile>.json); the web
// server just reads that state, so every tab sees the same data and responses are instant. The Manager owns
// one Collector per profile and polls only the selected one.
package collector

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/config"
	"github.com/ahmadarif-lab/router-monitoring/internal/drivers"
	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

const (
	historySecs  = 3600
	viewWindow   = 60 * time.Second // someone looked at the dashboard within this window -> poll fast
	fullBusy     = 15 * time.Second // client list / signal refresh interval while viewed
	fullIdle     = 45 * time.Second
	gapBusy      = 500 * time.Millisecond
	gapIdle      = 12 * time.Second
	maxSampleGap = 60.0 // seconds; a longer gap between counter readings is not averaged into a rate
	maxBackoff   = 60 * time.Second
	errAfter     = 2                 // consecutive failures before an error is shown
	RestartQuiet = 150 * time.Second // how long polling stops after a restart command
)

type Collector struct {
	store *config.Store
	id    string // profile id

	pollMu sync.Mutex // held while talking to the router (collector cycles, restart, login test)

	mu      sync.RWMutex
	snap    *router.Snapshot
	snapAt  time.Time
	lastErr *apperr.Error
	updated time.Time
	traffic Traffic
	drv     router.Driver
	drvSig  string

	restartUntil, quietUntil, pausedUntil time.Time

	fails     int
	wasPaused bool
	force     atomic.Bool
	lastSeen  atomic.Int64 // unix nanos of the last web request
	wake      chan struct{}
	done      chan struct{} // closed when Run has returned (state saved)
}

func NewCollector(s *config.Store, profileID string) *Collector {
	c := &Collector{store: s, id: profileID, wake: make(chan struct{}, 1)}
	c.loadState()
	return c
}

// ---- called by the web server ----

// Seen marks that someone is looking; wakes the loop if it was idling.
func (c *Collector) Seen() {
	prev := time.Unix(0, c.lastSeen.Swap(time.Now().UnixNano()))
	if time.Since(prev) > viewWindow {
		c.Wake()
	}
}

func (c *Collector) Wake() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// ForceRefresh asks for a full snapshot as soon as possible.
func (c *Collector) ForceRefresh() { c.force.Store(true); c.Wake() }

func (c *Collector) PauseFor(d time.Duration) {
	c.mu.Lock()
	if d <= 0 {
		c.pausedUntil = time.Time{}
	} else {
		c.pausedUntil = time.Now().Add(d)
	}
	c.mu.Unlock()
	c.Wake()
}

// StartRestartQuiet stops polling for the duration of a router restart.
func (c *Collector) StartRestartQuiet(d time.Duration) {
	c.mu.Lock()
	c.restartUntil = time.Now().Add(d)
	c.mu.Unlock()
}
func (c *Collector) ClearRestartQuiet() {
	c.mu.Lock()
	c.restartUntil = time.Time{}
	c.mu.Unlock()
}
func (c *Collector) QuietFor(d time.Duration) {
	c.mu.Lock()
	c.quietUntil = time.Now().Add(d)
	c.mu.Unlock()
}

// driver returns the driver for the current profile settings, rebuilding it when they changed.
func (c *Collector) driver() (router.Driver, config.Profile, error) {
	p, ok := c.store.Profile(c.id)
	if !ok || !p.Valid() {
		return nil, p, apperr.ErrNoProfile
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.drv == nil || c.drvSig != p.Signature() {
		if c.drv != nil {
			c.drv.Close()
		}
		d, err := drivers.New(p.Type, p.Creds())
		if err != nil {
			return nil, p, err
		}
		c.drv, c.drvSig = d, p.Signature()
		if sd, ok := d.(router.HistorySeeder); ok && len(c.traffic.Hist) == 0 {
			c.traffic.Hist = sd.SeedHistory(time.Now()) // demo drivers start with a filled graph
		}
	}
	return c.drv, p, nil
}

// Reboot restarts the router. It waits for a running poll to finish first.
func (c *Collector) Reboot(dry bool) error {
	d, _, err := c.driver()
	if err != nil {
		return err
	}
	c.pollMu.Lock()
	defer c.pollMu.Unlock()
	return d.Reboot(dry)
}

// Alive reports whether the router answers at all.
func (c *Collector) Alive() bool {
	d, _, err := c.driver()
	return err == nil && d.Alive()
}

// TestProfile checks a login without disturbing a running poll of the same profile.
func (c *Collector) TestProfile(p config.Profile) error {
	d, err := drivers.New(p.Type, p.Creds())
	if err != nil {
		return err
	}
	defer d.Close()
	c.pollMu.Lock()
	defer c.pollMu.Unlock()
	_, err = d.Poll(true)
	return err
}

// TestLogin checks the login of a profile that has no collector yet (a router being added).
func TestLogin(p config.Profile) error { return new(Collector).TestProfile(p) }

// ProfileChanged resets error state after the profile was edited.
func (c *Collector) ProfileChanged() {
	c.mu.Lock()
	c.fails, c.lastErr, c.snap, c.snapAt = 0, nil, nil, time.Time{}
	c.traffic = Traffic{}
	c.mu.Unlock()
	c.ForceRefresh()
}

// View is a consistent copy of the current state for the API.
type View struct {
	Snap       *router.Snapshot
	SnapAt     time.Time
	Err        *apperr.Error
	Updated    time.Time
	Hist       []router.Sample
	Restarting time.Duration
	Paused     time.Duration
}

func (c *Collector) View() View {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := time.Now()
	v := View{Snap: c.snap, SnapAt: c.snapAt, Err: c.lastErr, Updated: c.updated,
		Hist: append([]router.Sample{}, c.traffic.Hist...)}
	if d := c.restartUntil.Sub(now); d > 0 {
		v.Restarting = d
	}
	if d := c.pausedUntil.Sub(now); d > 0 {
		v.Paused = d
	}
	return v
}

// ---- loop ----

func (c *Collector) Run(ctx context.Context) {
	c.mu.Lock()
	c.done = make(chan struct{})
	done := c.done
	c.mu.Unlock()
	defer close(done)
	save := time.NewTicker(30 * time.Second)
	defer save.Stop()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-save.C:
				c.saveState()
			}
		}
	}()
	for ctx.Err() == nil {
		gap := c.step()
		t := time.NewTimer(gap)
		select {
		case <-ctx.Done():
			t.Stop()
		case <-c.wake:
			t.Stop()
		case <-t.C:
		}
	}
	c.saveState()
}

// Done returns a channel closed when the current Run has finished (nil if never started).
func (c *Collector) Done() <-chan struct{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.done
}

func (c *Collector) pauseReason(now time.Time) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	switch {
	case now.Before(c.restartUntil):
		return "restart"
	case now.Before(c.quietUntil):
		return "quiet"
	case now.Before(c.pausedUntil):
		return "paused"
	}
	return ""
}

func backoff(fails int) time.Duration {
	fails = max(fails, 1)
	d := 4 * time.Second << min(fails-1, 4) // 4, 8, 16, 32, 64 -> capped
	if d > maxBackoff {
		d = maxBackoff
	}
	return d
}

// step does one poll (if due) and returns how long to wait before the next one.
func (c *Collector) step() time.Duration {
	drv, _, err := c.driver()
	if err != nil {
		return time.Hour // woken when the profile changes
	}
	now := time.Now()
	viewed := now.UnixNano()-c.lastSeen.Load() < int64(viewWindow)
	gap, fullEvery := gapIdle, fullIdle
	if viewed {
		gap, fullEvery = max(gapBusy, drv.Interval()), fullBusy
	}

	c.pollMu.Lock()
	defer c.pollMu.Unlock()
	if c.pauseReason(now) != "" {
		c.wasPaused = true
		c.mu.Lock()
		c.updated = now
		c.mu.Unlock()
		return 2 * time.Second
	}
	if c.wasPaused { // router counters may have reset during the pause
		c.mu.Lock()
		c.traffic.Last = nil
		c.mu.Unlock()
		c.wasPaused = false
	}
	c.mu.RLock()
	full := c.force.Swap(false) || c.snap == nil || now.Sub(c.snapAt) >= fullEvery
	c.mu.RUnlock()

	if err := c.cycle(drv, full); err != nil {
		return max(gap, backoff(c.fails))
	}
	return gap
}

func (c *Collector) cycle(drv router.Driver, full bool) error {
	res, err := drv.Poll(!full)
	if err != nil {
		c.mu.Lock()
		c.fails++
		if c.fails >= errAfter {
			c.lastErr = apperr.From(err)
		}
		c.updated = time.Now()
		fails := c.fails
		c.mu.Unlock()
		if fails == errAfter {
			log.Printf("polling failed: %v", err)
		}
		return err
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fails >= errAfter {
		log.Printf("polling recovered")
	}
	c.fails, c.lastErr, c.updated = 0, nil, now
	at := res.At
	if at.IsZero() {
		at = now
	}
	t := float64(at.UnixNano()) / 1e9
	switch {
	case res.Counters != nil:
		c.traffic.Record(t, *res.Counters)
	case res.Speed != nil:
		c.traffic.RecordSpeed(t, *res.Speed)
	}
	if full && res.Snap != nil {
		c.snap, c.snapAt = res.Snap, now
	} else if c.snap != nil && res.Wan.Known() {
		next := *c.snap // copy-on-write: readers may hold the old snapshot
		next.Wan = res.Wan
		c.snap = &next
	}
	return nil
}
