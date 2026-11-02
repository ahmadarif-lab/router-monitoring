// Package restart restarts routers on demand and on a nightly schedule. The app does this itself, so no
// launchd/cron job or shell script is needed.
package restart

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/collector"
	"github.com/ahmadarif-lab/router-monitoring/internal/config"
)

const (
	waitDownTimeout = 60 * time.Second
	waitUpTimeout   = 5 * time.Minute
)

type tryState struct {
	day   string
	tries int
	next  time.Time
}

type Restarter struct {
	store *config.Store
	mgr   *collector.Manager

	mu    sync.Mutex
	busy  map[string]bool
	tries map[string]*tryState
}

func NewRestarter(s *config.Store, m *collector.Manager) *Restarter {
	return &Restarter{store: s, mgr: m, busy: map[string]bool{}, tries: map[string]*tryState{}}
}

// logf logs to stderr and appends a line to restart.log.
func (r *Restarter) logf(name, format string, a ...any) {
	msg := fmt.Sprintf("[%s] ", name) + fmt.Sprintf(format, a...)
	log.Print(msg)
	if f, err := os.OpenFile(r.store.Path("restart.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
		f.Close()
	}
}

func (r *Restarter) setResult(id string, res config.RestartResult) {
	if err := r.store.UpdateSchedule(id, func(s *config.Schedule) { s.LastRestart = &res }); err != nil {
		log.Printf("could not save restart result: %v", err)
	}
}

// Now sends the reboot command (or only checks the login when dry). It returns once the command
// is sent; waiting for the router to come back continues in the background.
func (r *Restarter) Now(id string, dry bool, source string) error {
	p, ok := r.store.Profile(id)
	if !ok || !p.Valid() {
		return apperr.ErrNoCreds
	}
	r.mu.Lock()
	if r.busy[id] {
		r.mu.Unlock()
		return apperr.ErrRestartBusy
	}
	r.busy[id] = true
	r.mu.Unlock()
	release := func() { r.mu.Lock(); delete(r.busy, id); r.mu.Unlock() }

	col := r.mgr.Collector(id)
	tag := p.Name + " · " + source
	if dry {
		col.QuietFor(20 * time.Second)
	} else {
		col.StartRestartQuiet(collector.RestartQuiet)
	}
	sent := time.Now()
	err := col.Reboot(dry)
	if err != nil {
		col.ClearRestartQuiet()
		r.logf(tag, "FAILED: %v", err)
		ae := apperr.From(err)
		r.setResult(id, config.RestartResult{Time: sent.Unix(), Source: source, Result: config.ResultFailed, Error: ae.Msg, Code: ae.Code})
		release()
		return err
	}
	if dry {
		r.logf(tag, "dry run OK: login worked, reboot NOT sent")
		release()
		return nil
	}
	r.logf(tag, "reboot command sent")
	go func() {
		defer release()
		r.waitBack(col, tag, id, source, sent)
	}()
	return nil
}

// waitBack verifies the router really went down and came back, and records how long it took.
func (r *Restarter) waitBack(col *collector.Collector, tag, id, source string, sent time.Time) {
	res := config.RestartResult{Time: sent.Unix(), Source: source}
	finish := func(result string, secs int) {
		res.Result, res.Secs = result, secs
		r.setResult(id, res)
		col.ClearRestartQuiet()
	}
	started := time.Now()
	down := false
	for time.Since(started) < waitDownTimeout {
		if !col.Alive() {
			down = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !down {
		r.logf(tag, "router still up %ds after the reboot command; the reboot was probably not triggered", int(waitDownTimeout.Seconds()))
		finish(config.ResultUnconfirmed, 0)
		return
	}
	for time.Since(started) < waitUpTimeout+waitDownTimeout {
		if col.Alive() {
			secs := int(time.Since(started).Seconds())
			r.logf(tag, "router back online after %ds", secs)
			finish(config.ResultOK, secs)
			return
		}
		time.Sleep(3 * time.Second)
	}
	r.logf(tag, "router did not come back within %d minutes", int(waitUpTimeout.Minutes()))
	finish(config.ResultOffline, 0)
}
