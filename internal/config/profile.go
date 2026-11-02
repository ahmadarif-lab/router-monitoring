// Package config holds what the user configures: the saved routers (profiles), their restart schedules and the
// device names. Everything lives in one data directory.
package config

import "github.com/ahmadarif-lab/router-monitoring/internal/router"

// Schedule is the nightly restart of one router, run by this app (no launchd/cron needed).
type Schedule struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`    // "HH:MM", local time
	LastRun string `json:"lastRun"` // "2006-01-02": date of the last attempt

	LastRestart *RestartResult `json:"lastRestart,omitempty"`
}

// Profile is one saved router: a home Biznet, an office GL.iNet, and so on.
type Profile struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"` // see DriverTypes
	URL      string   `json:"url"`
	User     string   `json:"user"`
	Pass     string   `json:"pass"` // stored in profiles.json (mode 0600), never sent to the browser
	Schedule Schedule `json:"schedule"`
}

func (p Profile) Creds() router.Credentials {
	return router.Credentials{URL: p.URL, User: p.User, Pass: p.Pass}
}
func (p Profile) Valid() bool { return p.ID != "" && p.Type != "" && p.Creds().Valid() }

// Signature changes whenever something that affects the router connection changes.
func (p Profile) Signature() string {
	return p.Type + "\x00" + p.URL + "\x00" + p.User + "\x00" + p.Pass
}

// ProfileInfo is what the browser may see (no password).
type ProfileInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	URL  string `json:"url"`
	User string `json:"user"`
}

func (p Profile) Info() ProfileInfo {
	return ProfileInfo{ID: p.ID, Name: p.Name, Type: p.Type, URL: p.URL, User: p.User}
}

// Restart outcomes shown in the UI.
const (
	ResultOK          = "ok"          // router went down and came back
	ResultFailed      = "failed"      // the reboot command could not be sent
	ResultUnconfirmed = "unconfirmed" // command sent but the router never went down
	ResultOffline     = "offline"     // router did not come back in time
)

// RestartResult is the last restart, stored in the profile and shown in the UI (translated there).
type RestartResult struct {
	Time   int64  `json:"time"`            // unix seconds when the command was sent
	Source string `json:"source"`          // "manual" or "schedule"
	Result string `json:"result"`          // one of the Result* constants
	Secs   int    `json:"secs,omitempty"`  // seconds until the router was back (resultOK)
	Error  string `json:"error,omitempty"` // English error message (resultFailed)
	Code   string `json:"code,omitempty"`  // error code, for translation
}

func defaultSchedule() Schedule { return Schedule{Time: "03:00"} }
