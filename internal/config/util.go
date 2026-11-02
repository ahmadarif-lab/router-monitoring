package config

import (
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
)

// NormalizeURL accepts "192.168.18.1" or "http://192.168.18.1/" and returns "http://192.168.18.1".
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", apperr.ErrURLEmpty
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", apperr.ErrURLInvalid
	}
	return u.Scheme + "://" + u.Host, nil
}

// ParseClock parses "HH:MM" into the time since midnight.
func ParseClock(v string) (time.Duration, error) {
	t, err := time.Parse("15:04", v)
	if err != nil {
		return 0, apperr.ErrBadTime
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
