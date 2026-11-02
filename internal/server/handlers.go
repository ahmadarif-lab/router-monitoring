package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/collector"
	"github.com/ahmadarif-lab/router-monitoring/internal/config"
	"github.com/ahmadarif-lab/router-monitoring/internal/drivers"
	"github.com/ahmadarif-lab/router-monitoring/internal/restart"
)

func (s *Server) handleAlias(w http.ResponseWriter, r *http.Request) {
	var b struct{ MAC, Name string }
	if !readBody(r, &b) || !macRe.MatchString(b.MAC) {
		fail(w, http.StatusBadRequest, apperr.ErrBadMAC)
		return
	}
	name := strings.TrimSpace(b.Name)
	if len([]rune(name)) > 60 {
		name = string([]rune(name)[:60])
	}
	if err := s.store.SetAlias(b.MAC, name); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Enabled bool   `json:"enabled"`
		Time    string `json:"time"`
	}
	if !readBody(r, &b) {
		fail(w, http.StatusBadRequest, apperr.ErrBadRequest)
		return
	}
	if b.Time != "" {
		if _, err := config.ParseClock(b.Time); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
	}
	prof, ok := s.store.Active()
	if !ok {
		fail(w, http.StatusBadRequest, apperr.ErrNoProfile)
		return
	}
	err := s.store.UpdateSchedule(prof.ID, func(sc *config.Schedule) {
		sc.Enabled = b.Enabled
		if b.Time != "" {
			sc.Time = b.Time
		}
		sc.LastRun = restart.SlotUsed(sc.Time, time.Now())
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	prof, _ = s.store.Profile(prof.ID)
	writeJSON(w, http.StatusOK, schedulePayload{Enabled: prof.Schedule.Enabled, Time: prof.Schedule.Time, Last: prof.Schedule.LastRestart})
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	var b struct{ Dry bool }
	_ = readBody(r, &b)
	prof, ok := s.store.Active()
	if !ok {
		fail(w, http.StatusBadRequest, apperr.ErrNoProfile)
		return
	}
	if err := s.rst.Now(prof.ID, b.Dry, "manual"); err != nil {
		fail(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dry": b.Dry})
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	var b struct{ Minutes int }
	_ = readBody(r, &b)
	b.Minutes = max(0, min(120, b.Minutes))
	if col, _, ok := s.mgr.Active(); ok {
		col.PauseFor(time.Duration(b.Minutes) * time.Minute)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "minutes": b.Minutes})
}

// handleProfile creates or updates a profile. The login is tested against the router first, and only a
// working login is saved. A blank password keeps the saved one.
func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	var b struct{ ID, Name, Type, URL, User, Pass string }
	if !readBody(r, &b) {
		fail(w, http.StatusBadRequest, apperr.ErrBadRequest)
		return
	}
	if !drivers.IsPublic(b.Type) {
		fail(w, http.StatusBadRequest, apperr.ErrBadType)
		return
	}
	base, err := config.NormalizeURL(b.URL)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	p := config.Profile{ID: b.ID, Name: strings.TrimSpace(b.Name), Type: b.Type, URL: base, User: strings.TrimSpace(b.User), Pass: b.Pass}
	var existing config.Profile
	if p.ID != "" {
		var ok bool
		if existing, ok = s.store.Profile(p.ID); !ok {
			fail(w, http.StatusNotFound, apperr.ErrNoProfile)
			return
		}
		if p.Pass == "" {
			p.Pass = existing.Pass
		}
	}
	if p.Name == "" {
		p.Name = drivers.Label(p.Type)
	}
	if p.User == "" || p.Pass == "" {
		fail(w, http.StatusBadRequest, apperr.ErrNeedUserPass)
		return
	}
	var testErr error
	if existing.ID != "" {
		testErr = s.mgr.Collector(existing.ID).TestProfile(p)
	} else {
		testErr = collector.TestLogin(p)
	}
	if err := testErr; err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	saved, err := s.store.SaveProfile(p)
	if err != nil {
		fail(w, http.StatusInternalServerError, apperr.New("save_failed", "Could not save: "+err.Error(), "detail", err.Error()))
		return
	}
	if existing.ID == "" {
		_ = s.mgr.Switch(saved.ID) // a new profile becomes the active one
	} else {
		s.mgr.Collector(saved.ID).ProfileChanged()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID})
}

func (s *Server) handleSwitch(w http.ResponseWriter, r *http.Request) {
	var b struct{ ID string }
	if !readBody(r, &b) {
		fail(w, http.StatusBadRequest, apperr.ErrBadRequest)
		return
	}
	if err := s.mgr.Switch(b.ID); err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var b struct{ ID string }
	if !readBody(r, &b) {
		fail(w, http.StatusBadRequest, apperr.ErrBadRequest)
		return
	}
	if err := s.store.DeleteProfile(b.ID); err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	s.mgr.Forget(b.ID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
