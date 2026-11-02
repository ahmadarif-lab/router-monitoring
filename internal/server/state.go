package server

import (
	"net/http"
	"net/url"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/config"
	"github.com/ahmadarif-lab/router-monitoring/internal/drivers"
	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

type schedulePayload struct {
	Enabled bool                  `json:"enabled"`
	Time    string                `json:"time"`
	Last    *config.RestartResult `json:"last"`
}

type statePayload struct {
	*router.Snapshot
	TS         int64                `json:"ts"`
	Error      string               `json:"error"`
	ErrorCode  string               `json:"errorCode,omitempty"`
	ErrorArgs  map[string]string    `json:"errorArgs,omitempty"`
	Restarting int                  `json:"restarting"`
	Paused     int                  `json:"paused"`
	Now        int64                `json:"now"`
	Age        float64              `json:"age"`
	History    []router.Sample      `json:"history"`
	Schedule   schedulePayload      `json:"schedule"`
	Configured bool                 `json:"configured"`
	Config     *config.ProfileInfo  `json:"config,omitempty"` // the active profile (url + user only, never the password)
	Profiles   []config.ProfileInfo `json:"profiles"`
	Active     string               `json:"active"`
	Suggest    string               `json:"suggest,omitempty"` // another profile whose router answers while the active one does not
	Types      []drivers.Info       `json:"types"`
	Version    string               `json:"version"`
}

// routerHost is the host part of a profile's address.
func routerHost(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Hostname()
	}
	return ""
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	p := statePayload{Now: time.Now().Unix(), Profiles: []config.ProfileInfo{}, History: []router.Sample{}, Types: drivers.Public(), Version: s.version}
	for _, pr := range s.store.Profiles() {
		p.Profiles = append(p.Profiles, pr.Info())
	}
	col, prof, ok := s.mgr.Active()
	if ok {
		if r.URL.Query().Get("force") == "1" {
			col.ForceRefresh()
		}
		col.Seen()
		info := prof.Info()
		p.Config, p.Active, p.Configured = &info, prof.ID, prof.Valid()
		p.Schedule = schedulePayload{Enabled: prof.Schedule.Enabled, Time: prof.Schedule.Time, Last: prof.Schedule.LastRestart}
		p.Suggest = s.mgr.Suggestion()
		v := col.View()
		p.Restarting, p.Paused, p.History = int(v.Restarting.Seconds()), int(v.Paused.Seconds()), v.Hist
		if v.Err != nil {
			p.Error, p.ErrorCode, p.ErrorArgs = v.Err.Msg, v.Err.Code, v.Err.Args
		}
		if !v.Updated.IsZero() {
			p.Age = time.Since(v.Updated).Round(100 * time.Millisecond).Seconds()
		}
		if v.Snap != nil {
			snap := *v.Snap
			aliases := s.store.Aliases()
			snap.Clients = append([]router.Client(nil), v.Snap.Clients...)
			for i := range snap.Clients {
				if a, ok := aliases[snap.Clients[i].MAC]; ok { // the user's own name wins over the router's
					snap.Clients[i].Alias = a
				}
			}
			markSelf(snap.Clients, r.RemoteAddr, routerHost(prof.URL), cachedLocalNets())
			p.Snapshot, p.TS = &snap, v.SnapAt.Unix()
		}
	} else {
		p.Schedule = schedulePayload{Time: "03:00"}
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	hist := []router.Sample{}
	if col, _, ok := s.mgr.Active(); ok {
		col.Seen()
		hist = col.View().Hist
	}
	writeJSON(w, http.StatusOK, map[string]any{"now": time.Now().Unix(), "history": hist})
}
