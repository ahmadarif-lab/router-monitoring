package huawei

// Low-level client for the Huawei EG8145V5 ONT web admin.
//
// The ONT allows ONE admin session at a time, so every poll is login -> fetch -> logout.
// Data pages are old ASP files that embed JavaScript constructors; they are parsed in parse.go.

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/netutil"
	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

var errNotHuawei = apperr.New("not_huawei", "Login page not recognized (is this a Huawei EG8145V5?)")

type Session struct {
	base string
	hc   *http.Client
}

func login(cr router.Credentials) (*Session, error) {
	jar, _ := cookiejar.New(nil)
	s := &Session{base: strings.TrimRight(cr.URL, "/"), hc: &http.Client{Jar: jar, Timeout: 15 * time.Second, Transport: netutil.Transport}}
	body, err := s.do("POST", "/asp/GetRandCount.asp", "", nil, nil)
	if err != nil {
		return nil, netutil.Unreachable(s.base, err)
	}
	tok := tokenRe.FindString(body)
	if tok == "" {
		return nil, errNotHuawei
	}
	form := url.Values{
		"UserName":     {cr.User},
		"PassWord":     {base64.StdEncoding.EncodeToString([]byte(cr.Pass))},
		"Language":     {"english"},
		"x.X_HW_Token": {tok},
	}
	_, err = s.do("POST", "/login.cgi", form.Encode(), map[string]string{
		"Referer": s.base + "/", "Cookie": "body:Language:english:id=-1",
	}, nil)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Session) do(method, path, body string, hdr map[string]string, hc *http.Client) (string, error) {
	req, err := http.NewRequest(method, s.base+path, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", netutil.UserAgent)
	req.Header.Set("Origin", s.base)
	if method == "POST" && body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if hc == nil {
		hc = s.hc
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return string(b), err
}

func (s *Session) page(path string) (string, error) {
	return s.do("GET", path, "", map[string]string{"Referer": s.base + "/index.asp"}, nil)
}

func (s *Session) Logout() {
	_, _ = s.do("POST", "/logout.cgi?RequestFile=html/logout.html", "", map[string]string{"Referer": s.base + "/index.asp"}, nil)
}

// ---------- fetching ----------

// raw is the unparsed text of the router pages.
type raw struct {
	Base                        string
	Dev, WLAN, Assoc, WAN, Info string
	WANAt                       time.Time // when the WAN counters were read (for accurate rates)
}

// fetch logs in, reads the pages, and logs out. light=true reads only the WAN page (traffic counters).
func fetch(cr router.Credentials, light bool) (*raw, error) {
	s, err := login(cr)
	if err != nil {
		return nil, err
	}
	defer s.Logout()
	raw := &raw{Base: s.base}
	if raw.WAN, err = s.page("/html/bbsp/common/wan_list.asp"); light || err != nil {
		raw.WANAt = time.Now()
		if err == nil && !looksLikeWANPage(raw.WAN) {
			err = apperr.ErrLoginRejected
		}
		return raw, err
	}
	raw.WANAt = time.Now()
	if raw.Dev, err = s.page("/html/bbsp/common/GetLanUserDevInfo.asp"); err != nil {
		return nil, err
	}
	if !strings.Contains(raw.Dev, "USERDevice") {
		return nil, apperr.ErrLoginRejected
	}
	if raw.WLAN, err = s.page("/html/amp/common/wlan_list.asp"); err != nil {
		return nil, err
	}
	// optional pages: failures only mean less detail
	raw.Assoc, _ = s.do("POST", "/html/amp/wlaninfo/getassociateddeviceinfo.asp", "", map[string]string{"Referer": s.base + "/index.asp"}, nil)
	raw.Info, _ = s.page("/html/ssmp/deviceinfo/deviceinfo.asp")
	return raw, nil
}

func looksLikeWANPage(s string) bool {
	for _, k := range []string{"WanEthPPStats", "WanEthIPStats", "PPPWanList", "WanPPP"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// ---------- reboot ----------

var resetTokenRe = regexp.MustCompile(`id="hwonttoken" value="([0-9a-f]*)"`)

// reboot logs in and sends the reset command. dry=true stops right before the reboot.
func reboot(cr router.Credentials, dry bool) error {
	s, err := login(cr)
	if err != nil {
		return err
	}
	page, err := s.page("/html/ssmp/reset/reset.asp")
	if err != nil {
		return err
	}
	m := resetTokenRe.FindStringSubmatch(page)
	if m == nil || m[1] == "" {
		s.Logout()
		return apperr.ErrLoginRejected
	}
	if dry {
		s.Logout()
		return nil
	}
	// The router may drop the connection while rebooting, so a transport error here is not a failure.
	_, _ = s.do("POST",
		"/html/ssmp/reset/set.cgi?x=InternetGatewayDevice.X_HW_DEBUG.SMP.DM.ResetBoard&RequestFile=html/ssmp/reset/reset.asp",
		url.Values{"x.X_HW_Token": {m[1]}}.Encode(),
		map[string]string{"Referer": s.base + "/html/ssmp/reset/reset.asp"}, nil)
	return nil
}
