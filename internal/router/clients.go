package router

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	androidRe = regexp.MustCompile(`^android-dhcp-(\d+)`)
	appleRe   = regexp.MustCompile(`iphone|ipad|macbook|-air|-pro|imac|mac-?mini|mac-?studio`)
	tvRe      = regexp.MustCompile(`tv|stb|chromecast|roku`)
)

// TypeRandomMAC is the Type value for devices with a locally administered (randomized) MAC.
const TypeRandomMAC = "randommac"

// VendorHint guesses the kind of device from its DHCP host name, DHCP vendor class and MAC address.
func VendorHint(host, vclass, mac string) string {
	h, v := strings.ToLower(host), strings.ToLower(vclass)
	if strings.HasPrefix(v, "msft") {
		return "Windows"
	}
	if m := androidRe.FindStringSubmatch(v); m != nil {
		return "Android " + m[1]
	}
	if appleRe.MatchString(h) {
		return "Apple"
	}
	if strings.HasPrefix(h, "lwip") || strings.HasPrefix(h, "esp") {
		return "IoT / ESP"
	}
	if tvRe.MatchString(h) {
		return "TV / STB"
	}
	if len(mac) >= 2 {
		if b, err := strconv.ParseUint(mac[:2], 16, 8); err == nil && b&2 != 0 {
			return TypeRandomMAC // phone using a randomized (private) MAC address; the UI translates this
		}
	}
	return ""
}

func ipKey(ip string) [4]int {
	k := [4]int{999, 0, 0, 0}
	if ip == "" {
		return k
	}
	for i, p := range strings.Split(ip, ".") {
		if i > 3 {
			break
		}
		k[i], _ = strconv.Atoi(p)
	}
	return k
}

// SortClients puts online clients first, then orders by IP address.
func SortClients(out []Client) {
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Online != out[j].Online {
			return out[i].Online
		}
		return lessKey(ipKey(out[i].IP), ipKey(out[j].IP))
	})
}

func lessKey(a, b [4]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
