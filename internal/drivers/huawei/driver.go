// Package huawei is the driver for the Biznet home router, a Huawei EG8145V5 ONT.
//
// It reads connected devices (with Wi-Fi signal and link rate), the WAN state and the WAN byte counters from
// the router's web pages, and can reboot it. It cannot read per-device traffic or CPU/memory.
package huawei

import (
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/netutil"
	"github.com/ahmadarif-lab/router-monitoring/internal/router"
)

type driver struct{ cr router.Credentials }

// New returns the driver for one router.
func New(cr router.Credentials) router.Driver { return &driver{cr: cr} }

func (d *driver) Poll(light bool) (*router.Result, error) {
	raw, err := fetch(d.cr, light)
	if err != nil {
		return nil, err
	}
	if light {
		wan, cnt := parseWAN(raw.WAN)
		return &router.Result{Wan: wan, Counters: cnt, At: raw.WANAt}, nil
	}
	snap, cnt := parseAll(raw)
	return &router.Result{Snap: snap, Wan: snap.Wan, Counters: cnt, At: raw.WANAt}, nil
}

func (d *driver) Reboot(dry bool) error   { return reboot(d.cr, dry) }
func (d *driver) Alive() bool             { return netutil.Alive(d.cr.URL) }
func (d *driver) Interval() time.Duration { return 0 } // the ONT needs ~4s per page anyway
func (d *driver) Close()                  {}
