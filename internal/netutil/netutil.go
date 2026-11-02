// Package netutil holds what the router drivers share when talking HTTP to a router on the LAN.
package netutil

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
)

// UserAgent is sent to routers; some web UIs refuse requests without a browser-like agent.
const UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/153.0.0.0 Safari/537.36"

// Transport fails fast when the router is unreachable (4s to connect) and never keeps connections
// open: an ONT's embedded web server has very few sockets, so every request uses a fresh connection.
var Transport = &http.Transport{
	DialContext:       (&net.Dialer{Timeout: 4 * time.Second}).DialContext,
	DisableKeepAlives: true,
}

// Alive reports whether anything answers HTTP at base (any status code counts).
func Alive(base string) bool {
	c := &http.Client{Timeout: 3 * time.Second, Transport: Transport}
	resp, err := c.Get(strings.TrimRight(base, "/") + "/")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// Reason classifies a network error into a short code (translated by the UI) plus the raw detail.
func Reason(err error) (reason, detail string) {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	detail = err.Error()
	var ne net.Error
	switch {
	case errors.As(err, &ne) && ne.Timeout():
		return "timeout", detail
	// Windows words these differently ("connectex: ... actively refused it"), so match both spellings.
	case errors.Is(err, syscall.ECONNREFUSED), strings.Contains(detail, "connection refused"), strings.Contains(detail, "actively refused"):
		return "refused", detail
	case strings.Contains(detail, "no route to host"), strings.Contains(detail, "network is unreachable"),
		strings.Contains(detail, "unreachable network"), strings.Contains(detail, "unreachable host"):
		return "noroute", detail
	case strings.Contains(detail, "no such host"):
		return "nohost", detail
	}
	return "other", detail
}

var reasonText = map[string]func(detail string) string{
	"timeout": func(string) string { return "no response, timed out" },
	"refused": func(string) string { return "connection refused, is this the router's address?" },
	"noroute": func(string) string { return "no route to that address, check your network" },
	"nohost":  func(string) string { return "host name not found" },
	"other":   func(d string) string { return d },
}

// Unreachable builds the "router not reachable" error for a failed request to base.
func Unreachable(base string, err error) *apperr.Error {
	reason, detail := Reason(err)
	return apperr.New("unreachable", fmt.Sprintf("Router not reachable at %s (%s)", base, reasonText[reason](detail)),
		"base", base, "reason", reason, "detail", detail)
}
