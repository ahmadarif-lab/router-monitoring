package server

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahmadarif-lab/router-monitoring/internal/collector"
	"github.com/ahmadarif-lab/router-monitoring/internal/config"
	"github.com/ahmadarif-lab/router-monitoring/internal/restart"
	"github.com/ahmadarif-lab/router-monitoring/internal/testrouter"
)

func newTestApp(t *testing.T) (*Server, *config.Store) {
	t.Helper()
	store, _ := config.OpenStore(t.TempDir())
	mgr := collector.NewManager(store)
	return New(store, mgr, restart.NewRestarter(store, mgr), []string{"router.lan"}, false, "test"), store
}

func req(s *Server, method, path, host, remote string, hdr map[string]string, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host, r.RemoteAddr = host, remote
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestGuard(t *testing.T) {
	s, _ := newTestApp(t)
	csrf := map[string]string{"X-Requested-With": "rm-ui"}
	if w := req(s, "GET", "/api/state", "localhost:8787", "127.0.0.1:5000", nil, ""); w.Code != 200 {
		t.Errorf("loopback+known host: %d", w.Code)
	}
	if w := req(s, "GET", "/api/state", "router.lan", "127.0.0.1:5000", nil, ""); w.Code != 200 {
		t.Errorf("extra host: %d", w.Code)
	}
	if w := req(s, "GET", "/api/state", "evil.example", "127.0.0.1:5000", nil, ""); w.Code != 403 {
		t.Errorf("foreign Host (DNS rebinding) must be 403, got %d", w.Code)
	}
	if w := req(s, "GET", "/api/state", "localhost", "192.168.18.50:5000", nil, ""); w.Code != 403 {
		t.Errorf("LAN client must be 403, got %d", w.Code)
	}
	if w := req(s, "POST", "/api/pause", "localhost", "127.0.0.1:5000", nil, `{"minutes":5}`); w.Code != 403 {
		t.Errorf("POST without header must be 403, got %d", w.Code)
	}
	cross := map[string]string{"X-Requested-With": "rm-ui", "Origin": "http://evil.example"}
	if w := req(s, "POST", "/api/pause", "localhost", "127.0.0.1:5000", cross, `{"minutes":5}`); w.Code != 403 {
		t.Errorf("cross-origin POST must be 403, got %d", w.Code)
	}
	if w := req(s, "POST", "/api/pause", "localhost", "127.0.0.1:5000", csrf, `{"minutes":5}`); w.Code != 200 {
		t.Errorf("valid POST: %d", w.Code)
	}
}

func TestProfileFlow(t *testing.T) {
	s, store := newTestApp(t)
	srv := testrouter.Huawei(t, "secret")
	csrf := map[string]string{"X-Requested-With": "rm-ui"}
	post := func(path, body string) *httptest.ResponseRecorder {
		return req(s, "POST", path, "localhost", "127.0.0.1:1", csrf, body)
	}
	if w := req(s, "GET", "/api/state", "localhost", "127.0.0.1:1", nil, ""); !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Errorf("fresh app must report configured:false: %s", w.Body.String())
	}
	bad := fmt.Sprintf(`{"type":"huawei","url":%q,"user":"admin","pass":"wrong"}`, srv.URL)
	if w := post("/api/profile", bad); w.Code != 400 || len(store.Profiles()) != 0 || !strings.Contains(w.Body.String(), `"code":"login_rejected"`) {
		t.Errorf("wrong password must be rejected (login_rejected) and not saved: %d %s", w.Code, w.Body.String())
	}
	if w := post("/api/profile", `{"type":"huawei","url":"","user":"admin","pass":"x"}`); w.Code != 400 || !strings.Contains(w.Body.String(), `"code":"url_empty"`) {
		t.Errorf("empty url: %d %s", w.Code, w.Body.String())
	}
	if w := post("/api/profile", `{"type":"demo-glinet","url":"x","user":"a","pass":"b"}`); w.Code != 400 {
		t.Errorf("demo types must not be creatable through the API: %d", w.Code)
	}
	good := fmt.Sprintf(`{"name":"Home","type":"huawei","url":%q,"user":"admin","pass":"secret"}`, srv.URL)
	w := post("/api/profile", good)
	if w.Code != 200 || len(store.Profiles()) != 1 {
		t.Fatalf("good login must be saved: %d %s", w.Code, w.Body.String())
	}
	id := store.Profiles()[0].ID
	state := req(s, "GET", "/api/state", "localhost", "127.0.0.1:1", nil, "").Body.String()
	if strings.Contains(state, "secret") {
		t.Error("API must never expose the password")
	}
	if strings.Contains(state, `"history":null`) || strings.Contains(state, `"profiles":null`) {
		t.Errorf("empty lists must be [] not null: %s", state)
	}
	if !strings.Contains(state, `"configured":true`) || !strings.Contains(state, `"active":"`+id+`"`) {
		t.Errorf("state after setup: %s", state)
	}
	// edit with a blank password keeps working; unknown id is a 404
	edit := fmt.Sprintf(`{"id":%q,"name":"Home 2","type":"huawei","url":%q,"user":"admin","pass":""}`, id, srv.URL)
	if w := post("/api/profile", edit); w.Code != 200 {
		t.Errorf("edit with blank password: %d %s", w.Code, w.Body.String())
	}
	if p, _ := store.Profile(id); p.Name != "Home 2" || p.Pass != "secret" {
		t.Errorf("edit result: %+v", p)
	}
	if w := post("/api/profile/switch", `{"id":"nope"}`); w.Code != 404 {
		t.Errorf("switch to unknown profile: %d", w.Code)
	}
	if w := post("/api/profile/delete", fmt.Sprintf(`{"id":%q}`, id)); w.Code != 200 || len(store.Profiles()) != 0 {
		t.Errorf("delete: %d", w.Code)
	}
}
