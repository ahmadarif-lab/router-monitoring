package glinet

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/router"
	"github.com/ahmadarif-lab/router-monitoring/internal/testrouter"
)

func TestGLiNetLightPoll(t *testing.T) {
	f := testrouter.GLiNet(t, "secret")
	d := New(router.Credentials{URL: f.URL, User: "root", Pass: "secret"})
	res, err := d.Poll(true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Speed == nil || res.Speed.Down != 125000 || res.Speed.Up != 12500 || res.Snap != nil {
		t.Fatalf("light poll: %+v", res)
	}
}

func TestGLiNetFullPoll(t *testing.T) {
	f := testrouter.GLiNet(t, "secret")
	d := New(router.Credentials{URL: f.URL, User: "root", Pass: "secret"})
	res, err := d.Poll(false)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Snap
	if s.Router.Vendor != "GL.iNet" || s.Router.Model != "GL-MT3000" || s.Router.FW != "4.9.0-op24" || s.Router.Uptime == nil || *s.Router.Uptime != 1615 {
		t.Errorf("router: %+v", s.Router)
	}
	if s.Wan.Online == nil || !*s.Wan.Online || s.Wan.Name != "wan" || s.Wan.Status != "Connected" {
		t.Errorf("wan: %+v", s.Wan)
	}
	if len(s.Clients) != 4 {
		t.Fatalf("clients: %d", len(s.Clients))
	}
	// online first, by IP: nas (cable), MacBook, guest phone, then the offline laptop
	order := []string{s.Clients[0].Host, s.Clients[1].Host, s.Clients[2].Host, s.Clients[3].Host}
	if strings.Join(order, ",") != "nas,MacBook-Pro,guest-phone,old-laptop" {
		t.Errorf("order: %v", order)
	}
	nas, mac, guest := s.Clients[0], s.Clients[1], s.Clients[2]
	if nas.Band != "lan" || nas.Conn != "LAN" {
		t.Errorf("wired client: %+v", nas)
	}
	if mac.Band != "5" || mac.Conn != "WiFi 5 GHz" || mac.SSID != "Office_5G" || mac.Alias != "Work Mac" || mac.MAC != "00:00:5e:00:53:01" {
		t.Errorf("5G client: %+v", mac)
	}
	if mac.Rate == nil || mac.Rate.Down != 172 || mac.Rate.Up != 5661 || mac.Total == nil || mac.Total.Down != 2033279813 {
		t.Errorf("client traffic (string and number totals): %+v %+v", mac.Rate, mac.Total)
	}
	if mac.Signal != nil {
		t.Errorf("GL.iNet reports no Wi-Fi signal (online_time is not a session length): %+v", mac.Signal)
	}
	if !guest.Guest || guest.SSID != "Office-Guest" || guest.Band != "2.4" {
		t.Errorf("guest client: %+v", guest)
	}
	if s.Clients[3].Rate != nil || s.Clients[3].Total == nil {
		t.Errorf("offline client has totals but no live rate: %+v", s.Clients[3])
	}
	sys := s.System
	if sys == nil || sys.CPUTemp == nil || *sys.CPUTemp != 61 || sys.MemTotal != 505085952 || sys.MemFree != 135327744 || len(sys.Load) != 3 || sys.FlashTotal != 268435456 {
		t.Errorf("system: %+v", sys)
	}
	if len(s.SSIDs) != 3 { // the one that is down is skipped
		t.Errorf("ssids: %+v", s.SSIDs)
	}
}

func TestGLiNetLoginAndSession(t *testing.T) {
	f := testrouter.GLiNet(t, "secret")
	bad := New(router.Credentials{URL: f.URL, User: "root", Pass: "wrong"})
	if _, err := bad.Poll(true); err != apperr.ErrLoginRejected {
		t.Errorf("wrong password must be apperr.ErrLoginRejected, got %v", err)
	}
	d := New(router.Credentials{URL: f.URL, User: "root", Pass: "secret"})
	for i := 0; i < 3; i++ {
		if _, err := d.Poll(true); err != nil {
			t.Fatal(err)
		}
	}
	if f.Logins() != 1 {
		t.Errorf("the session must be reused, got %d logins", f.Logins())
	}
	f.ExpireSession() // the router forgets the session: the next poll logs in again by itself
	if _, err := d.Poll(true); err != nil {
		t.Fatalf("expired session: %v", err)
	}
	if f.Logins() != 2 {
		t.Errorf("expected a transparent re-login, got %d logins", f.Logins())
	}
}

func TestGLiNetNotARouter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<html>some other device</html>") }))
	defer srv.Close()
	if _, err := New(router.Credentials{URL: srv.URL, User: "root", Pass: "x"}).Poll(true); err != errNotGLiNet {
		t.Errorf("want errNotGLiNet, got %v", err)
	}
	if _, err := New(router.Credentials{URL: "http://127.0.0.1:1", User: "root", Pass: "x"}).Poll(true); apperr.From(err).Code != "unreachable" {
		t.Errorf("want unreachable, got %v", err)
	}
}

func TestGLiNetReboot(t *testing.T) {
	f := testrouter.GLiNet(t, "secret")
	d := New(router.Credentials{URL: f.URL, User: "root", Pass: "secret"})
	if err := d.Reboot(true); err != nil || f.Rebooted() {
		t.Fatalf("dry run must only log in: %v rebooted=%v", err, f.Rebooted())
	}
	if err := d.Reboot(false); err != nil || !f.Rebooted() {
		t.Fatalf("reboot: %v rebooted=%v", err, f.Rebooted())
	}
}

func TestGLiNetRealResponses(t *testing.T) {
	f := testrouter.GLiNet(t, "secret")
	f.Fixtures = map[string]string{
		"clients.get_list": "glinet_clients.json", "system.get_status": "glinet_status.json",
		"system.get_info": "glinet_info.json", "clients.get_wan_speed": "glinet_wanspeed.json",
	}
	d := New(router.Credentials{URL: f.URL, User: "root", Pass: "secret"})
	res, err := d.Poll(false)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Snap
	if s.Router.Vendor != "GL.iNet" || s.Router.Model != "GL-MT3000" || s.Router.FW == "" || s.Router.Uptime == nil {
		t.Errorf("router: %+v", s.Router)
	}
	if s.Wan.Online == nil || !*s.Wan.Online || s.Wan.Name != "wan" {
		t.Errorf("the wired uplink must be picked: %+v", s.Wan)
	}
	if len(s.Clients) < 20 {
		t.Fatalf("expected the full client list, got %d", len(s.Clients))
	}
	var online, offline, guests, wired int
	for _, c := range s.Clients {
		if c.Online {
			online++
			if c.Rate == nil {
				t.Errorf("online client without live rate: %+v", c)
			}
		} else {
			offline++
			if c.Rate != nil {
				t.Errorf("offline client with live rate: %+v", c)
			}
		}
		if c.Guest {
			guests++
			if c.SSID != "Office-Guest" {
				t.Errorf("guest client SSID: %+v", c)
			}
		}
		if c.Band == "lan" {
			wired++
		}
		if c.Total == nil {
			t.Errorf("totals missing: %+v", c)
		}
		if !strings.HasPrefix(c.MAC, "00:00:5e:00:53:") {
			t.Errorf("fixture must be anonymized: %s", c.MAC)
		}
	}
	if online == 0 || offline == 0 || guests == 0 {
		t.Errorf("fixture should cover online/offline/guest clients: %d/%d/%d", online, offline, guests)
	}
	sys := s.System
	if sys == nil || sys.CPUTemp == nil || sys.MemTotal == 0 || sys.FlashTotal == 0 || len(sys.Load) != 3 {
		t.Errorf("system: %+v", sys)
	}
	if sys.MemFree+sys.MemCache > sys.MemTotal {
		t.Errorf("memory figures inconsistent: %+v", sys)
	}
	if res.Speed == nil || res.Speed.Down <= 0 {
		t.Errorf("wan speed: %+v", res.Speed)
	}
	if len(s.SSIDs) < 2 {
		t.Errorf("ssids: %+v", s.SSIDs)
	}
}
