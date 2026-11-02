package restart

import (
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/config"
)

const (
	restartWindow  = 30 * time.Minute // a missed schedule (laptop asleep) still runs if we wake within this window
	maxScheduleTry = 3
	retryAfter     = 2 * time.Minute
)

// SlotUsed returns the LastRun value to store when the schedule is saved. If today's time has already
// passed, today's slot counts as used: turning the switch on at 03:10 for "03:00" must not reboot the
// router right away. If the time is still ahead, it may run today.
func SlotUsed(clock string, now time.Time) string {
	at, err := config.ParseClock(clock)
	if err != nil {
		return ""
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(at)
	if start.After(now) {
		return ""
	}
	return now.Format("2006-01-02")
}

// scheduleDue reports whether the scheduled restart should run at time now.
func scheduleDue(sc config.Schedule, now time.Time, tries int, nextTry time.Time) bool {
	if !sc.Enabled {
		return false
	}
	at, err := config.ParseClock(sc.Time)
	if err != nil {
		return false
	}
	day := now.Format("2006-01-02")
	if sc.LastRun == day && tries == 0 {
		return false // already done today
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(at)
	if now.Before(start) || now.After(start.Add(restartWindow)) {
		return false
	}
	return tries == 0 || (tries < maxScheduleTry && now.After(nextTry))
}

// RunSchedule checks every 15s whether a profile's nightly restart is due.
func (r *Restarter) RunSchedule(stop <-chan struct{}) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		now := time.Now()
		day := now.Format("2006-01-02")
		for _, p := range r.store.Profiles() {
			r.mu.Lock()
			ts := r.tries[p.ID]
			if ts == nil || ts.day != day {
				ts = &tryState{day: day}
				r.tries[p.ID] = ts
			}
			tries, next, busy := ts.tries, ts.next, r.busy[p.ID]
			r.mu.Unlock()
			if busy || !p.Valid() || !scheduleDue(p.Schedule, now, tries, next) {
				continue
			}
			r.mu.Lock()
			ts.tries++
			ts.next = now.Add(retryAfter)
			r.mu.Unlock()
			_ = r.store.UpdateSchedule(p.ID, func(s *config.Schedule) { s.LastRun = day })
			if err := r.Now(p.ID, false, "schedule"); err != nil {
				r.logf(p.Name+" · schedule", "attempt %d failed, retrying shortly", tries+1)
			} else {
				r.mu.Lock()
				ts.tries = maxScheduleTry // success: stop retrying today
				r.mu.Unlock()
			}
		}
	}
}
