package testrouter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/unixcrypt"
)

// GLiNetServer is a fake GL.iNet router (firmware 4.x).
type GLiNetServer struct {
	*httptest.Server
	password string
	nonce    string
	mu       sync.Mutex
	sid      string       // the one valid session; empty = expired
	logins   atomic.Int32 // number of successful logins
	rebooted atomic.Bool
	// Fixtures maps "module.func" to a file in the test's testdata/ directory holding {"result": ...}; it is
	// checked before the built-in sample responses.
	Fixtures map[string]string
}

// GLiNet starts a fake GL.iNet router that accepts the user "root" with the given password.
func GLiNet(t *testing.T, password string) *GLiNetServer {
	t.Helper()
	f := &GLiNetServer{password: password, nonce: "n0nce-0123456789abcdef-0123456789"}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

// ExpireSession makes the router forget the current session, as it does after a timeout.
func (f *GLiNetServer) ExpireSession() { f.mu.Lock(); f.sid = ""; f.mu.Unlock() }

func (f *GLiNetServer) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/rpc" {
		fmt.Fprint(w, "<html>GL.iNet</html>")
		return
	}
	var in struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		ID     int             `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	reply := func(result any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": in.ID, "result": result})
	}
	denied := func() {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": in.ID, "error": map[string]any{"code": -32000, "message": "Access denied"}})
	}
	switch in.Method {
	case "challenge":
		reply(map[string]any{"hash-method": "sha256", "alg": 5, "salt": "saltsaltsalt1234", "nonce": f.nonce})
	case "login":
		var p struct{ Username, Hash string }
		_ = json.Unmarshal(in.Params, &p)
		cipher, _ := unixcrypt.Crypt(f.password, "$5$saltsaltsalt1234$")
		if p.Username != "root" || p.Hash != sha256hex("root:"+cipher+":"+f.nonce) {
			denied()
			return
		}
		f.logins.Add(1)
		f.mu.Lock()
		f.sid = fmt.Sprintf("sid-%d", f.logins.Load())
		sid := f.sid
		f.mu.Unlock()
		reply(map[string]any{"sid": sid, "username": "root"})
	case "call":
		var p []json.RawMessage
		_ = json.Unmarshal(in.Params, &p)
		var sid, module, fn string
		_ = json.Unmarshal(p[0], &sid)
		_ = json.Unmarshal(p[1], &module)
		_ = json.Unmarshal(p[2], &fn)
		f.mu.Lock()
		valid := f.sid != "" && sid == f.sid
		f.mu.Unlock()
		if !valid {
			denied()
			return
		}
		if file, ok := f.Fixtures[module+"."+fn]; ok {
			b, err := os.ReadFile("testdata/" + file)
			if err != nil {
				panic(err)
			}
			var fx struct {
				Result json.RawMessage `json:"result"`
			}
			_ = json.Unmarshal(b, &fx)
			reply(fx.Result)
			return
		}
		switch module + "." + fn {
		case "clients.get_wan_speed":
			reply(map[string]any{"speed_rx": 125000, "speed_tx": 12500}) // bytes/s: 1 Mbps down, 0.1 Mbps up
		case "clients.get_list":
			reply(map[string]any{"clients": []map[string]any{
				{"mac": "00:00:5E:00:53:02", "ip": "192.168.8.30", "name": "old-laptop", "alias": "", "online": false, "iface": "2.4G", "rx": 0, "tx": 0, "total_rx": "1361634", "total_tx": 25728738, "online_time": 0},
				{"mac": "00:00:5E:00:53:03", "ip": "192.168.8.12", "name": "guest-phone", "alias": "", "online": true, "iface": "2.4G_Guest", "rx": 100, "tx": 50, "total_rx": 2000, "total_tx": 1000, "online_time": time.Now().Unix() - 120},
				{"mac": "00:00:5e:00:53:01", "ip": "192.168.8.5", "name": "MacBook-Pro", "alias": "Work Mac", "online": true, "iface": "5G", "rx": 172, "tx": 5661, "total_rx": "2033279813", "total_tx": 1500, "online_time": time.Now().Unix() - 3600},
				{"mac": "00:00:5e:00:53:04", "ip": "192.168.8.2", "name": "nas", "online": true, "iface": "cable", "rx": 1, "tx": 2, "total_rx": 3, "total_tx": 4, "online_time": 0},
			}})
		case "system.get_status":
			reply(map[string]any{
				"system": map[string]any{
					"uptime": 1615.42, "memory_total": 505085952, "memory_free": 135327744, "memory_buff_cache": 102178816,
					"flash_total": 268435456, "flash_free": 140738560, "load_average": []float64{0.23, 0.15, 0.18}, "cpu": map[string]any{"temperature": 61},
				},
				"network": []map[string]any{{"interface": "wwan", "up": false, "online": true}, {"interface": "wan", "up": true, "online": true}},
				"wifi": []map[string]any{
					{"ssid": "Office", "band": "2G", "guest": false, "up": true}, {"ssid": "Office_5G", "band": "5G", "guest": false, "up": true},
					{"ssid": "Office-Guest", "band": "2G", "guest": true, "up": true}, {"ssid": "Off", "band": "5G", "guest": true, "up": false},
				},
			})
		case "system.get_info":
			reply(map[string]any{"vendor": "GL.iNet", "firmware_version": "4.9.0-op24", "board_info": map[string]any{"model": "GL.iNet GL-MT3000", "hostname": "GL-MT3000"}})
		case "system.reboot":
			f.rebooted.Store(true)
			reply(map[string]any{})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": in.ID, "error": map[string]any{"code": -32601, "message": "Method not found"}})
		}
	}
}

// Logins returns how many successful logins the router has seen.
func (f *GLiNetServer) Logins() int { return int(f.logins.Load()) }

// Rebooted reports whether a reboot command was received.
func (f *GLiNetServer) Rebooted() bool { return f.rebooted.Load() }

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
