package config

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
)

func TestProfilesRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Active(); ok {
		t.Fatal("fresh store has no active profile")
	}
	home, err := s.SaveProfile(Profile{Name: "Home", Type: "huawei", URL: "http://192.168.18.1", User: "admin", Pass: `p@ss "w#rd" $x\`})
	if err != nil {
		t.Fatal(err)
	}
	office, _ := s.SaveProfile(Profile{Name: "Office", Type: "glinet", URL: "http://192.168.8.1", User: "root", Pass: "secret"})
	if s.ActiveID() != office.ID {
		t.Error("a new profile becomes the active one")
	}
	if fi, _ := os.Stat(dir + "/profiles.json"); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("profiles.json mode = %v, want 0600 (it holds passwords)", fi.Mode().Perm())
	}
	// an edit with a blank password keeps the saved one and keeps the schedule
	_ = s.UpdateSchedule(home.ID, func(sc *Schedule) { sc.Enabled = true; sc.Time = "04:30" })
	if _, err := s.SaveProfile(Profile{ID: home.ID, Name: "Home Biznet", Type: "huawei", URL: "http://192.168.18.1", User: "admin"}); err != nil {
		t.Fatal(err)
	}
	s2, _ := OpenStore(dir) // reload from disk
	got, ok := s2.Profile(home.ID)
	if !ok || got.Name != "Home Biznet" || got.Pass != home.Pass || !got.Schedule.Enabled || got.Schedule.Time != "04:30" {
		t.Errorf("reload/edit lost data: %+v", got)
	}
	if s2.ActiveID() != office.ID || len(s2.Profiles()) != 2 {
		t.Errorf("active/profiles after reload: %s %d", s2.ActiveID(), len(s2.Profiles()))
	}
	if err := s2.DeleteProfile(office.ID); err != nil || s2.ActiveID() != home.ID {
		t.Errorf("deleting the active profile must fall back to another: %v %s", err, s2.ActiveID())
	}
	if err := s2.DeleteProfile("nope"); err != apperr.ErrNoProfile {
		t.Errorf("deleting an unknown profile: %v", err)
	}
	if info, _ := json.Marshal(got.Info()); strings.Contains(string(info), "pass") {
		t.Error("ProfileInfo must not carry the password")
	}
}

func TestNormalizeURL(t *testing.T) {
	for in, want := range map[string]string{"192.168.18.1": "http://192.168.18.1", " http://192.168.18.1/ ": "http://192.168.18.1", "https://r.lan:8443/x": "https://r.lan:8443"} {
		if got, err := NormalizeURL(in); err != nil || got != want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x", "http://"} {
		if _, err := NormalizeURL(bad); err == nil {
			t.Errorf("NormalizeURL(%q) should fail", bad)
		}
	}
}

// ---- fake router: mimics the Huawei login flow + pages ----
