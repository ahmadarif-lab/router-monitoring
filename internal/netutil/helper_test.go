package netutil

import "net/http"

type httpGet struct{}

func (*httpGet) get(u string) (*http.Response, error) {
	return (&http.Client{Transport: Transport}).Get(u)
}
