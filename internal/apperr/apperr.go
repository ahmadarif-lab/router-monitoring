// Package apperr defines errors that carry a stable machine-readable code.
//
// The server sends the English message plus the code (and optional arguments); the web UI translates the
// message by code, so Go code never needs to know about languages.
package apperr

import "errors"

// Error is an error with a code and optional arguments for the translation.
type Error struct {
	Code string            `json:"code"`
	Msg  string            `json:"message"`
	Args map[string]string `json:"args,omitempty"`
}

func (e *Error) Error() string { return e.Msg }

// New builds an Error; kv is a list of alternating argument names and values.
func New(code, msg string, kv ...string) *Error {
	e := &Error{Code: code, Msg: msg}
	if len(kv) > 1 {
		e.Args = map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			e.Args[kv[i]] = kv[i+1]
		}
	}
	return e
}

// From converts any error into an *Error (unknown errors get the generic code "error"). nil stays nil.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: "error", Msg: err.Error()}
}

// Errors shared by several packages. Driver-specific ones live in the driver's package.
var (
	ErrLoginRejected = New("login_rejected", "Login rejected: check the username and password, or another admin session is active")
	ErrBadTime       = New("bad_time", "Time must be in HH:MM format")
	ErrNoCreds       = New("not_configured", "Router credentials are not set yet")
	ErrRestartBusy   = New("restart_busy", "A restart is already in progress")
	ErrBadRequest    = New("bad_request", "Invalid request")
	ErrBadMAC        = New("bad_mac", "Invalid MAC address")
	ErrNeedUserPass  = New("creds_required", "Username and password are required")
	ErrURLEmpty      = New("url_empty", "Router address is empty")
	ErrURLInvalid    = New("url_invalid", "Invalid router address (example: http://192.168.18.1)")
	ErrNoProfile     = New("no_profile", "Profile not found")
	ErrBadType       = New("bad_type", "Unknown router type")
)
