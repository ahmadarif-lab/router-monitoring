// Package demo has drivers with invented data, for `-demo`, for screenshots, and for trying the app without a
// router. All names, IPs and MAC addresses are made up (MACs use the documentation range 00:00:5e:00:53:xx, RFC 7042).
package demo

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

type demoDriver struct {
	glinet bool // GL.iNet style (system card, per-client traffic) or Huawei style (Wi-Fi signal)
	start  time.Time

	mu        sync.Mutex
	downUntil time.Time
	last      time.Time
	cum       router.Counters
}

// NewGLiNet returns a demo router that looks like a GL.iNet (system card, per-client traffic).
func NewGLiNet() router.Driver { return newDemo(true) }

// NewHuawei returns a demo router that looks like the Biznet/Huawei ONT (Wi-Fi signal and link rates).
func NewHuawei() router.Driver { return newDemo(false) }

func newDemo(glinet bool) *demoDriver {
	return &demoDriver{glinet: glinet, start: time.Now().Add(-3*time.Hour - 25*time.Minute)}
}

// wanRates is a plausible-looking WAN speed (Mbps) at time t: a slow wave plus occasional bursts.
func wanRates(t float64) (up, down float64) {
	slow := 0.5 + 0.5*math.Sin(t/170)
	burst := math.Max(0, math.Sin(t/61)) * math.Pow(math.Max(0, math.Sin(t/453+1)), 3)
	down = 1.5 + 6*slow + 70*burst + 0.8*math.Sin(t/7)
	up = 0.25 + 0.6*slow + 6*burst + 0.1*math.Sin(t/5)
	return max(up, 0.05), max(down, 0.1)
}

// SeedHistory gives a new demo collector half an hour of graph so the chart is never empty.
func (d *demoDriver) SeedHistory(now time.Time) []router.Sample {
	var out []router.Sample
	for t := now.Add(-30 * time.Minute); t.Before(now); t = t.Add(5 * time.Second) {
		u, dn := wanRates(float64(t.Unix()))
		out = append(out, router.Sample{float64(t.Unix()), math.Round(u*100) / 100, math.Round(dn*100) / 100})
	}
	return out
}

func (d *demoDriver) Interval() time.Duration { return 2 * time.Second }
func (d *demoDriver) Close()                  {}
func (d *demoDriver) Alive() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return time.Now().After(d.downUntil)
}

func (d *demoDriver) Reboot(dry bool) error {
	if !dry {
		d.mu.Lock()
		d.downUntil = time.Now().Add(8 * time.Second)
		d.mu.Unlock()
	}
	return nil
}

func (d *demoDriver) Poll(light bool) (*router.Result, error) {
	now := time.Now()
	d.mu.Lock()
	if now.Before(d.downUntil) {
		d.mu.Unlock()
		return nil, apperr.New("unreachable", "Router not reachable (demo reboot)", "base", "demo", "reason", "timeout", "detail", "")
	}
	up, down := wanRates(float64(now.Unix()))
	if !d.last.IsZero() {
		dt := now.Sub(d.last).Seconds()
		d.cum.Up += uint64(up * 1e6 / 8 * dt)
		d.cum.Down += uint64(down * 1e6 / 8 * dt)
	}
	d.last = now
	cum := d.cum
	d.mu.Unlock()

	res := &router.Result{At: now}
	if d.glinet {
		res.Speed = &router.Speed{Up: up * 1e6 / 8, Down: down * 1e6 / 8}
	} else {
		res.Counters = &cum
	}
	if light {
		return res, nil
	}
	if d.glinet {
		res.Snap = d.glSnapshot(now, up, down)
	} else {
		res.Snap = d.huaweiSnapshot(now)
	}
	res.Wan = res.Snap.Wan
	return res, nil
}

// demoSelf is the made-up device the demo presents as "this device". Real drivers never set Client.Self: the server
// does, per request, by looking at the computer that opens the dashboard.
const demoSelf = "Studio-MacBook"

func mac(n int) string { return fmt.Sprintf("00:00:5e:00:53:%02x", n) }
func ip(n int) string  { return fmt.Sprintf("192.168.1.%d", n) }
func wave(now time.Time, period, amp float64) float64 {
	return math.Sin(float64(now.Unix())/period) * amp
}
func iptr(n int) *int { return &n }

func (d *demoDriver) huaweiSnapshot(now time.Time) *router.Snapshot {
	type dev struct {
		host, ssidBand, mode, typ string
		n, rssi, tx, rx           int
		online                    bool
	}
	devs := []dev{
		{"Living-Room-TV", "5", "11ac", "TV / STB", 10, -49, 866, 866, true},
		{"Alex-iPhone", "5", "11ax", "Apple", 11, -57, 1200, 1200, true},
		{"Maya-Pixel-8", "5", "11ax", router.TypeRandomMAC, 12, -63, 780, 650, true},
		{"Studio-MacBook", "5", "11ax", "Apple", 13, -45, 1201, 1201, true},
		{"Kitchen-Speaker", "2.4", "11n", "IoT / ESP", 14, -66, 72, 72, true},
		{"Work-Laptop", "2.4", "11n", "Windows", 15, -71, 144, 144, true},
		{"Garage-Camera", "2.4", "11n", "IoT / ESP", 16, -78, 65, 65, true},
		{"Guest-Tablet", "2.4", "11n", "Android 14", 17, -74, 58, 58, false},
		{"Old-Printer", "2.4", "11g", "", 18, -80, 54, 54, false},
	}
	var cl []router.Client
	for _, v := range devs {
		c := router.Client{IP: ip(v.n), MAC: mac(v.n), Host: v.host, Online: v.online, DHCP: true, Mode: v.mode, Type: v.typ}
		c.Self = v.host == demoSelf
		if v.ssidBand == "5" {
			c.Conn, c.Band, c.SSID = "WiFi 5 GHz", "5", "Home_5G"
		} else {
			c.Conn, c.Band, c.SSID = "WiFi 2.4 GHz", "2.4", "Home"
		}
		if v.online {
			rssi := v.rssi + int(math.Round(wave(now, float64(7+v.n), 2)))
			up := (v.n*4373)%40000 + 1200 + int(now.Sub(d.start).Seconds())%60
			c.Signal = &router.Signal{RSSI: iptr(rssi), SNR: iptr(rssi + 95), TX: iptr(v.tx), RX: iptr(v.rx), Uptime: iptr(up)}
		}
		cl = append(cl, c)
	}
	up := int(now.Sub(d.start).Seconds())
	on := true
	return &router.Snapshot{
		Clients: cl,
		Router:  router.RouterInfo{Vendor: "Huawei", Model: "EG8145V5", FW: "V5R020C10S115", IP: "192.168.1.1", Uptime: &up},
		Wan:     router.Wan{Online: &on, Status: "Connected", IP: "203.0.113.42", GW: "203.0.113.1", Name: "pppoe"},
		SSIDs:   []router.SSIDInfo{{SSID: "Home", Band: "2.4GHz"}, {SSID: "Home_5G", Band: "5GHz"}},
	}
}

func (d *demoDriver) glSnapshot(now time.Time, upMbps, downMbps float64) *router.Snapshot {
	type dev struct {
		host, band, typ string
		n               int
		down, up        float64 // current Mbps
		totDown, totUp  float64 // GB
		online, guest   bool
	}
	devs := []dev{
		{"Studio-MacBook", "5", "Apple", 20, downMbps * 0.55, upMbps * 0.5, 38.2, 12.7, true, false},
		{"Alex-iPhone", "5", "Apple", 21, downMbps * 0.2, upMbps * 0.1, 21.4, 3.1, true, false},
		{"Living-Room-TV", "5", "TV / STB", 22, downMbps * 0.15, 0.02, 64.8, 0.9, true, false},
		{"Maya-Pixel-8", "5", router.TypeRandomMAC, 23, downMbps * 0.07, upMbps * 0.3, 9.6, 2.2, true, false},
		{"Desk-Lamp", "2.4", "IoT / ESP", 24, 0.01, 0.01, 0.1, 0.1, true, false},
		{"Visitor-Phone", "2.4", "Android 15", 25, 0.3, 0.05, 1.8, 0.4, true, true},
		{"Spare-Laptop", "2.4", "Windows", 26, 0, 0, 5.5, 0.7, false, false},
	}
	var cl []router.Client
	for _, v := range devs {
		c := router.Client{IP: ip(v.n), MAC: mac(v.n), Host: v.host, Online: v.online, DHCP: true, Type: v.typ, Guest: v.guest}
		c.Self = v.host == demoSelf
		if v.band == "5" {
			c.Conn, c.Band, c.SSID = "WiFi 5 GHz", "5", "Office_5G"
		} else {
			c.Conn, c.Band, c.SSID = "WiFi 2.4 GHz", "2.4", "Office"
		}
		if v.guest {
			c.SSID = "Office-Guest"
		}
		c.Total = &router.Total{Down: uint64(v.totDown * 1e9), Up: uint64(v.totUp * 1e9)}
		if v.online {
			c.Rate = &router.Rate{Down: v.down * 1e6 / 8, Up: v.up * 1e6 / 8}
		}
		cl = append(cl, c)
	}
	up := int(now.Sub(d.start).Seconds())
	on := true
	temp := 52 + wave(now, 40, 3)
	free := uint64(float64(210<<20) + wave(now, 90, 6e6))
	return &router.Snapshot{
		Clients: cl,
		Router:  router.RouterInfo{Vendor: "GL.iNet", Model: "GL-MT3000", FW: "4.9.0", IP: "192.168.1.1", Hostname: "GL-MT3000", Uptime: &up},
		Wan:     router.Wan{Online: &on, Status: "Connected", Name: "wan"},
		SSIDs:   []router.SSIDInfo{{SSID: "Office", Band: "2.4GHz"}, {SSID: "Office_5G", Band: "5GHz"}, {SSID: "Office-Guest", Band: "2.4GHz", Guest: true}},
		System: &router.System{
			Load: []float64{0.31 + math.Abs(wave(now, 50, 0.2)), 0.27, 0.22}, CPUTemp: &temp,
			MemTotal: 505 << 20, MemFree: free, MemCache: 96 << 20, FlashTotal: 256 << 20, FlashFree: 134 << 20,
		},
	}
}
