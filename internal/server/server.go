// Package server is the web UI and its JSON API. It only reads the collectors' state, plus a few admin
// actions (restart, pause, profiles, device names); it never talks to a router itself.
package server

import (
	"embed"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/collector"
	"github.com/ahmadarif-lab/router-monitoring/internal/config"
	"github.com/ahmadarif-lab/router-monitoring/internal/restart"
)

//go:embed web/index.html
var webFS embed.FS

var macRe = regexp.MustCompile(`^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$`)

type Server struct {
	store   *config.Store
	mgr     *collector.Manager
	rst     *restart.Restarter
	hosts   map[string]bool // allowed Host header names (without port)
	lan     bool            // allow non-loopback clients (no login of its own: use only on a trusted network)
	version string
	index   []byte
}

// New builds the server. extraHosts are additional allowed Host header names; lan allows non-loopback clients.
func New(s *config.Store, m *collector.Manager, r *restart.Restarter, extraHosts []string, lan bool, version string) *Server {
	idx, _ := webFS.ReadFile("web/index.html")
	h := map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}
	for _, x := range extraHosts {
		if x = strings.ToLower(strings.TrimSpace(x)); x != "" {
			h[x] = true
		}
	}
	return &Server{store: s, mgr: m, rst: r, hosts: h, lan: lan, version: version, index: idx}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.guard(false, s.handleIndex))
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /api/state", s.guard(false, s.handleState))
	mux.HandleFunc("GET /api/clients", s.guard(false, s.handleState))
	mux.HandleFunc("GET /api/traffic", s.guard(false, s.handleTraffic))
	mux.HandleFunc("POST /api/alias", s.guard(true, s.handleAlias))
	mux.HandleFunc("POST /api/schedule", s.guard(true, s.handleSchedule))
	mux.HandleFunc("POST /api/restart", s.guard(true, s.handleRestart))
	mux.HandleFunc("POST /api/pause", s.guard(true, s.handlePause))
	mux.HandleFunc("POST /api/profile", s.guard(true, s.handleProfile))
	mux.HandleFunc("POST /api/profile/switch", s.guard(true, s.handleSwitch))
	mux.HandleFunc("POST /api/profile/delete", s.guard(true, s.handleDelete))
	return mux
}

// guard protects the admin actions: there is no login of its own, so only local use is allowed by default.
//   - the client must be loopback (unless -lan) and the Host header a known name (anti DNS-rebinding)
//   - writes need the custom X-Requested-With header and a same-origin Origin (anti CSRF)
func (s *Server) guard(write bool, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.lan {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if p := net.ParseIP(ip); p == nil || !p.IsLoopback() || !s.hosts[hostOnly(r.Host)] {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		if write {
			if r.Header.Get("X-Requested-With") != "rm-ui" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if o := r.Header.Get("Origin"); o != "" {
				if u, err := url.Parse(o); err != nil || !strings.EqualFold(u.Host, r.Host) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		h(w, r)
	}
}

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// fail writes {"error": message, "code": code, "args": {...}} so the UI can translate by code.
func fail(w http.ResponseWriter, status int, err error) {
	ae := apperr.From(err)
	writeJSON(w, status, map[string]any{"error": ae.Msg, "code": ae.Code, "args": ae.Args})
}

func readBody(r *http.Request, v any) bool {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<16)).Decode(v) == nil
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(s.index)
}
