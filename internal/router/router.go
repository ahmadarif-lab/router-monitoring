// Package router is the contract between the router drivers and the rest of the app: the data model every
// driver fills in, and the Driver interface. A driver fills what its router can tell and leaves the rest
// empty; the web UI hides what is missing.
package router

import "time"

// The data model shared by all router drivers and the web UI. A driver fills in what its router
// can tell; everything else stays nil/zero and the UI hides it.

// Signal is the Wi-Fi link of one client (only some routers report it).
type Signal struct {
	RSSI   *int `json:"rssi"`
	SNR    *int `json:"snr"`
	RX     *int `json:"rx"` // negotiated link rate, Mbps
	TX     *int `json:"tx"`
	Uptime *int `json:"uptime"` // seconds connected
}

// Rate is the current traffic of one client in bytes per second, and Total the bytes since counters began.
type Rate struct {
	Down float64 `json:"down"`
	Up   float64 `json:"up"`
}
type Total struct {
	Down uint64 `json:"down"`
	Up   uint64 `json:"up"`
}

type Client struct {
	IP     string  `json:"ip"`
	MAC    string  `json:"mac"`
	Host   string  `json:"host"`
	Online bool    `json:"online"`
	Conn   string  `json:"conn"` // "WiFi 5 GHz", or "LAN 1" for wired
	Band   string  `json:"band"` // "2.4", "5" or "lan"
	SSID   string  `json:"ssid"`
	Mode   string  `json:"mode"`
	Type   string  `json:"type"`
	Alias  string  `json:"alias"`
	Signal *Signal `json:"signal"`
	Rate   *Rate   `json:"rate,omitempty"`  // routers that meter clients
	Total  *Total  `json:"total,omitempty"` // routers that meter clients
	DHCP   bool    `json:"dhcp"`
	Lease  int     `json:"lease"`
	Guest  bool    `json:"guest,omitempty"`
	Self   bool    `json:"self,omitempty"` // the computer using the dashboard (set by the server per request)
}

type Wan struct {
	Online *bool  `json:"online"`
	Status string `json:"status"`
	IP     string `json:"ip"`
	GW     string `json:"gw"`
	DNS    string `json:"dns"`
	Name   string `json:"name"`
}

// Known reports whether the driver could tell anything about the WAN.
func (w Wan) Known() bool { return w.Online != nil || w.IP != "" || w.Status != "" }

type RouterInfo struct {
	Vendor   string `json:"vendor"`
	Model    string `json:"model"`
	FW       string `json:"fw"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname,omitempty"`
	Uptime   *int   `json:"uptime"`
}

type SSIDInfo struct {
	SSID  string `json:"ssid"`
	Band  string `json:"band"`
	Guest bool   `json:"guest,omitempty"`
}

// System is the router's own health: CPU, memory and flash.
type System struct {
	Load       []float64 `json:"load,omitempty"` // 1, 5 and 15 minute load average
	Cores      int       `json:"cores,omitempty"`
	CPUPct     *float64  `json:"cpuPct,omitempty"`
	CPUTemp    *float64  `json:"cpuTemp,omitempty"` // °C
	MemTotal   uint64    `json:"memTotal,omitempty"`
	MemFree    uint64    `json:"memFree,omitempty"`
	MemCache   uint64    `json:"memCache,omitempty"`
	FlashTotal uint64    `json:"flashTotal,omitempty"`
	FlashFree  uint64    `json:"flashFree,omitempty"`
}

type Snapshot struct {
	Clients []Client   `json:"clients"`
	Router  RouterInfo `json:"router"`
	Wan     Wan        `json:"wan"`
	SSIDs   []SSIDInfo `json:"ssids"`
	System  *System    `json:"system,omitempty"`
}

// Sample is one point of the traffic graph: unix time, upload Mbps, download Mbps. It marshals as [t,u,d].
type Sample [3]float64

// HistorySeeder is implemented by drivers that can provide a graph for a brand-new collector (the demo drivers).
type HistorySeeder interface {
	SeedHistory(now time.Time) []Sample
}

// Counters are cumulative WAN byte counters (32-bit on the Huawei ONT, they wrap).
type Counters struct{ Up, Down uint64 }

// Speed is the current WAN speed in bytes per second, for routers that report it directly.
type Speed struct{ Up, Down float64 }

// Result is one poll. Snap is set on full polls only; Counters or Speed carry the traffic figure.
type Result struct {
	Snap     *Snapshot
	Wan      Wan
	Counters *Counters
	Speed    *Speed
	At       time.Time // when the traffic figure was read
}

// Driver talks to one kind of router. One instance serves one profile.
type Driver interface {
	// Poll reads the router. light=true reads only what the traffic graph needs.
	Poll(light bool) (*Result, error)
	// Reboot restarts the router; dry=true only verifies that the login works.
	Reboot(dry bool) error
	// Alive reports whether the router answers at all (used while it reboots, and to suggest a profile).
	Alive() bool
	// Interval is the minimum time between two polls while someone is watching.
	Interval() time.Duration
	// Close releases the session.
	Close()
}

// Credentials are the login of one router.
type Credentials struct {
	URL  string `json:"url"`
	User string `json:"user"`
	Pass string `json:"-"`
}

// Valid reports whether all three fields are set.
func (c Credentials) Valid() bool { return c.URL != "" && c.User != "" && c.Pass != "" }
