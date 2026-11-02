// Package glinet is the driver for GL.iNet routers (firmware 4.x). The web UI talks JSON-RPC to /rpc:
//
//	challenge {username}                     -> {alg, salt, nonce, "hash-method"}
//	login     {username, hash}               -> {sid}      hash = H(username:crypt(password, "$alg$salt$"):nonce)
//	call      [sid, module, func, params]    -> module result
//
// Unlike the Huawei ONT, a GL.iNet allows several sessions and answers in milliseconds, and it reports the
// current WAN speed, the traffic of each client and its own CPU/memory directly.
package glinet

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/netutil"
	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

type driver struct {
	cr   router.Credentials
	base string
	hc   *http.Client

	mu     sync.Mutex // one RPC conversation at a time
	sid    string
	nextID int
	info   *glInfo
	infoAt time.Time
}

// New returns the driver for one router.
func New(cr router.Credentials) router.Driver {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 4 * time.Second}).DialContext,
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConnsPerHost: 2,
	}
	return &driver{cr: cr, base: strings.TrimRight(cr.URL, "/"), hc: &http.Client{Timeout: 15 * time.Second, Transport: tr}}
}

func (g *driver) Interval() time.Duration { return 2 * time.Second }
func (g *driver) Close()                  { g.hc.CloseIdleConnections() }

func (g *driver) Alive() bool {
	return netutil.Alive(g.base)
}

func (g *driver) Poll(light bool) (*router.Result, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var sp struct {
		Rx flexNum `json:"speed_rx"`
		Tx flexNum `json:"speed_tx"`
	}
	if err := g.call("clients", "get_wan_speed", nil, &sp); err != nil {
		return nil, err
	}
	res := &router.Result{Speed: &router.Speed{Up: float64(sp.Tx), Down: float64(sp.Rx)}, At: time.Now()}
	if light {
		return res, nil
	}
	var cl struct {
		Clients []glClient `json:"clients"`
	}
	if err := g.call("clients", "get_list", nil, &cl); err != nil {
		return nil, err
	}
	var st glStatus
	if err := g.call("system", "get_status", nil, &st); err != nil {
		return nil, err
	}
	if g.info == nil || time.Since(g.infoAt) > 10*time.Minute { // model and firmware rarely change
		var in glInfo
		if err := g.call("system", "get_info", nil, &in); err == nil {
			g.info, g.infoAt = &in, time.Now()
		}
	}
	res.Snap = buildGLSnapshot(g.base, cl.Clients, st, g.info, time.Now())
	res.Wan = res.Snap.Wan
	return res, nil
}

func (g *driver) Reboot(dry bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sid == "" {
		if err := g.login(); err != nil {
			return err
		}
	}
	if dry {
		return nil
	}
	// The router drops the connection while rebooting, so a transport error here is not a failure.
	err := g.call("system", "reboot", map[string]any{"delay": 1}, nil)
	if ae := apperr.From(err); ae != nil && ae.Code == "unreachable" {
		return nil
	}
	return err
}
