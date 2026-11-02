package drivers

import (
	"testing"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

func TestRegistry(t *testing.T) {
	for _, ty := range []string{Huawei, GLiNet, DemoHuawei, DemoGLiNet} {
		if d, err := New(ty, router.Credentials{URL: "http://x", User: "u", Pass: "p"}); err != nil || d == nil {
			t.Errorf("driver %s: %v", ty, err)
		}
	}
	if _, err := New("nope", router.Credentials{}); err != apperr.ErrBadType {
		t.Errorf("unknown type: %v", err)
	}
	if !IsPublic(Huawei) || !IsPublic(GLiNet) || IsPublic(DemoGLiNet) || IsPublic("nope") {
		t.Error("only real router types may be added by users")
	}
	if Label(GLiNet) != "GL.iNet" || Label("nope") != "" {
		t.Error("labels")
	}
}
