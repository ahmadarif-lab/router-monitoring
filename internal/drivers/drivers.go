// Package drivers is the registry of supported router types. Adding a router means writing a driver package
// that implements router.Driver and listing it here.
package drivers

import (
	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/drivers/demo"
	"github.com/ahmadarif-lab/router-monitoring/internal/drivers/glinet"
	"github.com/ahmadarif-lab/router-monitoring/internal/drivers/huawei"
	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

// Router type identifiers, as stored in profiles.
const (
	Huawei     = "huawei"
	GLiNet     = "glinet"
	DemoHuawei = "demo-huawei"
	DemoGLiNet = "demo-glinet"
)

// Info describes a router type for the profile editor.
type Info struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	DefaultURL  string `json:"url"`
	DefaultUser string `json:"user"`
	Hidden      bool   `json:"-"` // demo types are not offered in the editor
}

var all = []Info{
	{Type: Huawei, Label: "Biznet / Huawei EG8145V5", DefaultURL: "http://192.168.18.1"},
	{Type: GLiNet, Label: "GL.iNet", DefaultURL: "http://192.168.8.1", DefaultUser: "root"},
	{Type: DemoHuawei, Label: "Demo (Huawei style)", Hidden: true},
	{Type: DemoGLiNet, Label: "Demo (GL.iNet style)", Hidden: true},
}

// Public lists the router types a user may add.
func Public() []Info {
	var out []Info
	for _, i := range all {
		if !i.Hidden {
			out = append(out, i)
		}
	}
	return out
}

// IsPublic reports whether typ is a type a user may add.
func IsPublic(typ string) bool {
	for _, i := range Public() {
		if i.Type == typ {
			return true
		}
	}
	return false
}

// Label returns the display name of a router type ("" if unknown).
func Label(typ string) string {
	for _, i := range all {
		if i.Type == typ {
			return i.Label
		}
	}
	return ""
}

// New builds the driver for a router type.
func New(typ string, cr router.Credentials) (router.Driver, error) {
	switch typ {
	case Huawei:
		return huawei.New(cr), nil
	case GLiNet:
		return glinet.New(cr), nil
	case DemoHuawei:
		return demo.NewHuawei(), nil
	case DemoGLiNet:
		return demo.NewGLiNet(), nil
	}
	return nil, apperr.ErrBadType
}
