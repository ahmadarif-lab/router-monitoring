package huawei

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

// ---------- parsing ----------

var (
	tokenRe  = regexp.MustCompile(`[0-9a-f]{32,}`)
	quotedRe = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
	hexEsc   = regexp.MustCompile(`\\x([0-9a-fA-F]{2})`)

	argBlob       = `\(((?:"(?:[^"\\]|\\.)*",?)+)\)`
	userDeviceRe  = regexp.MustCompile(`new USERDevice` + argBlob)
	assocRe       = regexp.MustCompile(`new stAssociatedDevice` + argBlob)
	wanPPPRe      = regexp.MustCompile(`new WanPPP` + argBlob)
	wanStatsPPPRe = regexp.MustCompile(`WanEthPPStats = new Array\(new WaninfoStats` + argBlob)
	wanStatsIPRe  = regexp.MustCompile(`WanEthIPStats = new Array\(new WaninfoStats` + argBlob)
	devInfoRe     = regexp.MustCompile(`new stDeviceInfo` + argBlob)
	uptimeRe      = regexp.MustCompile(`dev_uptime = '(\d+)'`)
	workModeRe    = regexp.MustCompile(`new stWifiWorkingMode\("[^"]*","([^"]*)","[^"]*","([^"]*)"\)`)
	wlanInfoRe    = regexp.MustCompile(`new stWlanInfo\("[^"]*WLANConfiguration\\x2e(\d+)","[^"]*","([^"]*)","(\d+)","(\d+)","([^"]*)"\)`)
	ssidPortRe    = regexp.MustCompile(`^SSID(\d+)$`)
	numRe         = regexp.MustCompile(`^-?\d+$`)
)

// unesc decodes \xNN escapes; the bytes are read as UTF-8 when valid, else Latin-1.
func unesc(s string) string {
	if !strings.Contains(s, `\x`) {
		return s
	}
	out := hexEsc.ReplaceAllFunc([]byte(s), func(m []byte) []byte {
		v, _ := strconv.ParseUint(string(m[2:]), 16, 8)
		return []byte{byte(v)}
	})
	if utf8.Valid(out) {
		return string(out)
	}
	rs := make([]rune, len(out))
	for i, c := range out {
		rs[i] = rune(c)
	}
	return string(rs)
}

func argsOf(blob string) []string {
	var out []string
	for _, m := range quotedRe.FindAllStringSubmatch(blob, -1) {
		out = append(out, unesc(m[1]))
	}
	return out
}

func num(v string) *int {
	if !numRe.MatchString(v) {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}

// router.TypeRandomMAC is the Type value for devices with a locally administered (randomized) MAC.
func parseClients(dev string, ssids map[string][2]string, assoc string) []router.Client {
	modes := map[string]string{}
	for _, m := range workModeRe.FindAllStringSubmatch(dev, -1) {
		modes[strings.ToLower(unesc(m[2]))] = m[1]
	}
	sig := map[string]*router.Signal{}
	for _, m := range assocRe.FindAllStringSubmatch(assoc, -1) {
		a := argsOf(m[1])
		if len(a) < 9 {
			continue
		}
		sig[strings.ToLower(a[1])] = &router.Signal{RSSI: num(a[5]), SNR: num(a[7]), RX: num(a[3]), TX: num(a[4]), Uptime: num(a[2])}
	}
	seen := map[string]bool{}
	out := []router.Client{}
	for _, m := range userDeviceRe.FindAllStringSubmatch(dev, -1) {
		a := argsOf(m[1])
		if len(a) < 15 || seen[a[0]] {
			continue
		}
		seen[a[0]] = true
		ip, mac, port, iptype, vclass, status, ptype, host := a[1], strings.ToLower(a[2]), a[3], a[4], a[5], a[6], a[7], a[9]
		lease := a[len(a)-1]
		c := router.Client{MAC: mac, Online: status == "Online", Mode: modes[mac], DHCP: iptype == "DHCP"}
		if ip != "--" {
			c.IP = ip
		}
		if host != "--" {
			c.Host = host
		}
		if ptype == "WIFI" {
			if sm := ssidPortRe.FindStringSubmatch(port); sm != nil {
				info, ok := ssids[sm[1]]
				if !ok {
					band := "2.4GHz"
					if n, _ := strconv.Atoi(sm[1]); n >= 5 {
						band = "5GHz"
					}
					info = [2]string{"SSID" + sm[1], band}
				}
				c.Conn = "WiFi " + strings.Replace(info[1], "GHz", " GHz", 1)
				c.SSID = info[0]
				c.Band = "2.4"
				if strings.Contains(info[1][:min(2, len(info[1]))], "5") {
					c.Band = "5"
				}
			}
		}
		if c.Conn == "" {
			c.Conn = strings.Replace(port, "LAN", "LAN ", 1) // wired: "LAN 1" (the UI adds the translated "Wired")
			c.Band = "lan"
		}
		c.Type = router.VendorHint(host, vclass, mac)
		if c.Online {
			c.Signal = sig[mac]
		}
		if n, err := strconv.Atoi(lease); err == nil {
			c.Lease = n
		}
		out = append(out, c)
	}
	router.SortClients(out)
	return out
}

// sortClients puts online clients first, then orders by IP address.
func parseWAN(text string) (router.Wan, *router.Counters) {
	var w router.Wan
	if m := wanPPPRe.FindStringSubmatch(text); m != nil {
		if a := argsOf(m[1]); len(a) > 17 {
			on := a[4] == "Connected"
			w = router.Wan{Online: &on, Status: a[4], Name: a[7], IP: a[13], GW: a[14], DNS: a[17]}
		}
	}
	m := wanStatsPPPRe.FindStringSubmatch(text)
	if m == nil {
		m = wanStatsIPRe.FindStringSubmatch(text)
	}
	if m != nil {
		if a := argsOf(m[1]); len(a) > 2 {
			up, e1 := strconv.ParseUint(a[1], 10, 64)
			down, e2 := strconv.ParseUint(a[2], 10, 64)
			if e1 == nil && e2 == nil {
				return w, &router.Counters{Up: up, Down: down} // field 1 = sent (upload), field 2 = received (download)
			}
		}
	}
	return w, nil
}

func parseAll(raw *raw) (*router.Snapshot, *router.Counters) {
	ssids := map[string][2]string{}
	list := []router.SSIDInfo{}
	for _, m := range wlanInfoRe.FindAllStringSubmatch(raw.WLAN, -1) {
		ssid, band := unesc(m[2]), unesc(m[5])
		ssids[m[1]] = [2]string{ssid, band}
		if m[3] == "1" && m[4] == "1" {
			list = append(list, router.SSIDInfo{SSID: ssid, Band: band})
		}
	}
	wan, cnt := parseWAN(raw.WAN)
	host := raw.Base
	if u, err := url.Parse(raw.Base); err == nil {
		host = u.Hostname()
	}
	ri := router.RouterInfo{Vendor: "Huawei", Model: "EG8145V5", IP: host}
	if m := devInfoRe.FindStringSubmatch(raw.Info); m != nil {
		if a := argsOf(m[1]); len(a) > 4 {
			ri.FW, ri.Model = a[3], a[4]
		}
	}
	if m := uptimeRe.FindStringSubmatch(raw.Info); m != nil {
		ri.Uptime = num(m[1])
	}
	return &router.Snapshot{Clients: parseClients(raw.Dev, ssids, raw.Assoc), Router: ri, Wan: wan, SSIDs: list}, cnt
}

// delta returns the byte difference between two 32-bit counter readings, handling wrap-around.
