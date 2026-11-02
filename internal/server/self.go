package server

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

// localNets describes this computer's network interfaces: each address with its subnet, and the MAC addresses.
type localNets struct {
	nets []*net.IPNet // IP is the interface address, the mask its subnet
	macs map[string]bool
}

func readLocalNets() localNets {
	ln := localNets{macs: map[string]bool{}}
	ifs, err := net.Interfaces()
	if err != nil {
		return ln
	}
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		if len(ifc.HardwareAddr) == 6 {
			ln.macs[strings.ToLower(ifc.HardwareAddr.String())] = true
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				ln.nets = append(ln.nets, n)
			}
		}
	}
	return ln
}

var (
	localMu    sync.Mutex
	localCache localNets
	localAt    time.Time
)

// cachedLocalNets re-reads the interfaces at most every 30 seconds (Wi-Fi changes are rare).
func cachedLocalNets() localNets {
	localMu.Lock()
	defer localMu.Unlock()
	if time.Since(localAt) > 30*time.Second {
		localCache, localAt = readLocalNets(), time.Now()
	}
	return localCache
}

// markSelf flags the client that is the computer using the dashboard, so the user can see their own IP on the
// router. remote is the request's remote address. A request from this computer (loopback) means the computer
// running the app: it matches by MAC address, or by IP when the router is on that interface's subnet (so an
// address that merely coincides on another network is not mistaken for this computer). A request from another
// device (the -lan option) means that device: it matches by its IP.
func markSelf(clients []router.Client, remote, routerHost string, local localNets) {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	rip := net.ParseIP(host)
	if rip != nil && !rip.IsLoopback() {
		for i := range clients {
			if ip := net.ParseIP(clients[i].IP); ip != nil && ip.Equal(rip) {
				clients[i].Self = true
			}
		}
		return
	}
	routerIP := net.ParseIP(routerHost)
	for i := range clients {
		c := &clients[i]
		if !c.Online {
			continue
		}
		if local.macs[strings.ToLower(c.MAC)] {
			c.Self = true
			continue
		}
		cip := net.ParseIP(c.IP)
		if cip == nil {
			continue
		}
		for _, n := range local.nets {
			if n.IP.Equal(cip) && (routerIP == nil || n.Contains(routerIP)) {
				c.Self = true
			}
		}
	}
}
