package config

import (
	"encoding/json"
	"os"
	"strings"
)

// Aliases are device names, editable by hand: {"aa:bb:cc:dd:ee:ff": "Living room TV"} ----

func (s *Store) Aliases() map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(s.Path("aliases.json"))
	if err != nil {
		return out
	}
	var raw map[string]string
	if json.Unmarshal(b, &raw) != nil {
		return out
	}
	for k, v := range raw {
		if !strings.HasPrefix(k, "_") {
			out[strings.ToLower(k)] = v
		}
	}
	return out
}

func (s *Store) SetAlias(mac, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := map[string]string{}
	if b, err := os.ReadFile(s.Path("aliases.json")); err == nil {
		_ = json.Unmarshal(b, &data)
	}
	mac = strings.ToLower(mac)
	for k := range data {
		if strings.ToLower(k) == mac {
			delete(data, k)
		}
	}
	if name != "" {
		data[mac] = name
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	return writeFileAtomic(s.Path("aliases.json"), b, 0o644)
}
