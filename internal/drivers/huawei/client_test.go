package huawei

import (
	"testing"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/router"
	"github.com/ahmadarif-lab/router-monitoring/internal/testrouter"
)

func TestFetchAgainstFakeRouter(t *testing.T) {
	srv := testrouter.Huawei(t, "secret")
	good := router.Credentials{URL: srv.URL, User: "admin", Pass: "secret"}

	raw, err := fetch(good, false)
	if err != nil {
		t.Fatal(err)
	}
	snap, cnt := parseAll(raw)
	if len(snap.Clients) != 1 || snap.Clients[0].SSID != "HOME" || snap.Router.Model != "EG8145V5" || *snap.Router.Uptime != 4242 {
		t.Errorf("snapshot: %+v", snap)
	}
	if snap.Wan.IP != "10.1.2.3" || cnt == nil || cnt.Down != 200 {
		t.Errorf("wan: %+v %+v", snap.Wan, cnt)
	}
	if _, err := fetch(good, true); err != nil {
		t.Errorf("light fetch: %v", err)
	}
	if _, err := fetch(router.Credentials{URL: srv.URL, User: "admin", Pass: "wrong"}, true); err != apperr.ErrLoginRejected {
		t.Errorf("wrong password should be apperr.ErrLoginRejected, got %v", err)
	}
	if err := reboot(good, true); err != nil { // dry-run: token found, reboot not sent
		t.Errorf("dry reboot: %v", err)
	}
}
