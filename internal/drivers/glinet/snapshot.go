package glinet

// Mapping of the router's JSON to the shared data model.

import (
	"net/url"
	"strings"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

type glClient struct {
	MAC     string  `json:"mac"`
	IP      string  `json:"ip"`
	Name    string  `json:"name"`
	Alias   string  `json:"alias"`
	Online  bool    `json:"online"`
	Iface   string  `json:"iface"`
	Rx      flexNum `json:"rx"`
	Tx      flexNum `json:"tx"`
	TotalRx flexNum `json:"total_rx"`
	TotalTx flexNum `json:"total_tx"`
}

type glStatus struct {
	System struct {
		Uptime      flexNum   `json:"uptime"`
		MemoryTotal flexNum   `json:"memory_total"`
		MemoryFree  flexNum   `json:"memory_free"`
		MemoryCache flexNum   `json:"memory_buff_cache"`
		FlashTotal  flexNum   `json:"flash_total"`
		FlashFree   flexNum   `json:"flash_free"`
		Load        []float64 `json:"load_average"`
		CPU         struct {
			Temperature *float64 `json:"temperature"`
		} `json:"cpu"`
	} `json:"system"`
	Network []struct {
		Interface string `json:"interface"`
		Up        bool   `json:"up"`
		Online    bool   `json:"online"`
	} `json:"network"`
	WiFi []struct {
		SSID  string `json:"ssid"`
		Band  string `json:"band"`
		Guest bool   `json:"guest"`
		Up    bool   `json:"up"`
	} `json:"wifi"`
}

type glInfo struct {
	Vendor          string `json:"vendor"`
	FirmwareVersion string `json:"firmware_version"`
	BoardInfo       struct {
		Model    string `json:"model"`
		Hostname string `json:"hostname"`
	} `json:"board_info"`
}

// ---------- mapping ----------

var glBandName = map[string]string{"2G": "2.4GHz", "5G": "5GHz", "6G": "6GHz"}

// glConn classifies an interface name such as "5G", "2.4G_Guest" or "cable".
func glConn(iface string) (conn, band string, guest bool) {
	guest = strings.Contains(iface, "Guest")
	switch {
	case strings.HasPrefix(iface, "2.4G"):
		return "WiFi 2.4 GHz", "2.4", guest
	case strings.HasPrefix(iface, "5G"):
		return "WiFi 5 GHz", "5", guest
	case strings.HasPrefix(iface, "6G"):
		return "WiFi 6 GHz", "6", guest
	}
	return "LAN", "lan", false
}

func buildGLSnapshot(base string, clients []glClient, st glStatus, info *glInfo, now time.Time) *router.Snapshot {
	// SSID names by band and guest flag, to label each client's network
	type bk struct {
		band  string
		guest bool
	}
	ssids := map[bk]string{}
	var ssidList []router.SSIDInfo
	for _, w := range st.WiFi {
		if !w.Up {
			continue
		}
		band := glBandName[w.Band]
		ssids[bk{strings.TrimSuffix(w.Band, "G"), w.Guest}] = w.SSID
		ssidList = append(ssidList, router.SSIDInfo{SSID: w.SSID, Band: band, Guest: w.Guest})
	}
	out := make([]router.Client, 0, len(clients))
	for _, c := range clients {
		conn, band, guest := glConn(c.Iface)
		mac := strings.ToLower(c.MAC)
		cl := router.Client{
			IP: c.IP, MAC: mac, Host: c.Name, Alias: c.Alias, Online: c.Online,
			Conn: conn, Band: band, Guest: guest, DHCP: true, Type: router.VendorHint(c.Name, "", mac),
		}
		if band == "2.4" || band == "5" || band == "6" {
			key := band
			if band == "2.4" {
				key = "2"
			}
			cl.SSID = ssids[bk{key, guest}]
		}
		if c.Online {
			cl.Rate = &router.Rate{Down: float64(c.Rx), Up: float64(c.Tx)}
		}
		cl.Total = &router.Total{Down: uint64(c.TotalRx), Up: uint64(c.TotalTx)}
		out = append(out, cl)
	}
	router.SortClients(out)

	snap := &router.Snapshot{Clients: out, SSIDs: ssidList}
	host := base
	if u, err := url.Parse(base); err == nil {
		host = u.Hostname()
	}
	snap.Router = router.RouterInfo{Vendor: "GL.iNet", IP: host}
	if info != nil {
		if info.Vendor != "" {
			snap.Router.Vendor = info.Vendor
		}
		snap.Router.Model = strings.TrimSpace(strings.TrimPrefix(info.BoardInfo.Model, snap.Router.Vendor))
		snap.Router.FW, snap.Router.Hostname = info.FirmwareVersion, info.BoardInfo.Hostname
	}
	if st.System.Uptime > 0 {
		u := int(st.System.Uptime)
		snap.Router.Uptime = &u
	}

	// WAN: the first connected uplink in priority order (cable, repeater, tethering, modem)
	var wan *struct {
		Interface string `json:"interface"`
		Up        bool   `json:"up"`
		Online    bool   `json:"online"`
	}
	for _, name := range []string{"wan", "wwan", "tethering", "modem"} {
		for i := range st.Network {
			if st.Network[i].Interface == name && st.Network[i].Up {
				wan = &st.Network[i]
				break
			}
		}
		if wan != nil {
			break
		}
	}
	if wan != nil {
		on := wan.Online
		snap.Wan = router.Wan{Online: &on, Name: wan.Interface, Status: map[bool]string{true: "Connected", false: "Disconnected"}[on]}
	} else {
		off := false
		snap.Wan = router.Wan{Online: &off, Status: "Disconnected"}
	}

	s := st.System
	sys := &router.System{
		Load: s.Load, CPUTemp: s.CPU.Temperature,
		MemTotal: uint64(s.MemoryTotal), MemFree: uint64(s.MemoryFree), MemCache: uint64(s.MemoryCache),
		FlashTotal: uint64(s.FlashTotal), FlashFree: uint64(s.FlashFree),
	}
	snap.System = sys
	return snap
}
