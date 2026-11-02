package netutil

import (
	"errors"
	"strings"
	"testing"
)

func TestUnreachableRefused(t *testing.T) {
	_, err := (&httpGet{}).get("http://127.0.0.1:1")
	ae := Unreachable("http://127.0.0.1:1", err)
	if ae.Code != "unreachable" || ae.Args["reason"] != "refused" || ae.Args["base"] != "http://127.0.0.1:1" {
		t.Fatalf("wrong error: %+v", ae)
	}
	if strings.Contains(ae.Msg, "dial tcp") {
		t.Errorf("message should be user friendly, got %q", ae.Msg)
	}
}

func TestReasonClassification(t *testing.T) {
	for msg, want := range map[string]string{
		"dial tcp 10.0.0.1:80: connectex: No connection could be made because the target machine actively refused it.": "refused",
		"dial tcp 10.0.0.1:80: connect: no route to host":                                                              "noroute",
		"dial tcp: lookup router.lan: no such host":                                                                    "nohost",
		"something odd": "other",
	} {
		if got, _ := Reason(errors.New(msg)); got != want {
			t.Errorf("Reason(%q) = %q, want %q", msg, got, want)
		}
	}
}

func TestAliveFalseWhenNothingListens(t *testing.T) {
	if Alive("http://127.0.0.1:1") {
		t.Error("nothing listens on port 1")
	}
}
