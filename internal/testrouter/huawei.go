// Package testrouter has fake routers for tests: a Huawei ONT web admin and a GL.iNet JSON-RPC server. They
// speak the real login protocols, so drivers, collectors and the HTTP server can be tested end to end
// without hardware.
package testrouter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// HuaweiDevice builds one `new USERDevice(...)` entry as the ONT's client page would contain it.
func HuaweiDevice(ip, mac, port, vclass, status, ptype, host string) string {
	return fmt.Sprintf(`new USERDevice("d%s","%s","%s","%s","DHCP","%s","%s","%s","t","%s","a","b","c","d","e","3600")`,
		mac, ip, mac, port, vclass, status, ptype, host)
}

// Huawei starts a fake Huawei EG8145V5 web admin: token + login flow, data pages and the reset page.
// It accepts the user "admin" with the given password.
func Huawei(t *testing.T, pass string) *httptest.Server {
	t.Helper()
	wan := `new WanPPP("a","b","c","d","Connected","f","g","ppp_1","i","j","k","l","m","10.1.2.3","10.1.2.1","p","q","8.8.8.8");` +
		`var WanEthPPStats = new Array(new WaninfoStats("x","100","200","y"),null);`
	mux := http.NewServeMux()
	mux.HandleFunc("/asp/GetRandCount.asp", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "0123456789abcdef0123456789abcdef0123")
	})
	mux.HandleFunc("/login.cgi", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("UserName") == "admin" && r.Form.Get("PassWord") == "c2VjcmV0" && pass == "secret" { // base64("secret")
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "ok", Path: "/"})
		}
		fmt.Fprint(w, "<html>login</html>")
	})
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie("sid"); err != nil || c.Value != "ok" {
				fmt.Fprint(w, "<html>Waiting... top.location.replace</html>")
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/html/bbsp/common/wan_list.asp", authed(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, wan) }))
	mux.HandleFunc("/html/bbsp/common/GetLanUserDevInfo.asp", authed(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, HuaweiDevice("192.168.18.10", "68:5e:dd:6f:fb:d9", "SSID1", "", "Online", "WIFI", "Air"))
	}))
	mux.HandleFunc("/html/amp/common/wlan_list.asp", authed(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `new stWlanInfo("InternetGatewayDevice.LANDevice.1.WLANConfiguration\x2e1","x","HOME","1","1","2.4GHz")`)
	}))
	mux.HandleFunc("/html/amp/wlaninfo/getassociateddeviceinfo.asp", authed(func(w http.ResponseWriter, r *http.Request) {}))
	mux.HandleFunc("/html/ssmp/deviceinfo/deviceinfo.asp", authed(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `var dev_uptime = '4242'; new stDeviceInfo("a","b","c","V5R019","EG8145V5")`)
	}))
	mux.HandleFunc("/html/ssmp/reset/reset.asp", authed(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<input id="hwonttoken" value="abcdef0123">`)
	}))
	mux.HandleFunc("/logout.cgi", func(w http.ResponseWriter, r *http.Request) {})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}
