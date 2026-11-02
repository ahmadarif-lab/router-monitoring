package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
)

type profilesFile struct {
	Active   string    `json:"active"`
	Profiles []Profile `json:"profiles"`
}

// Store holds everything the user can configure; all files live in one data directory.
type Store struct {
	Dir string

	mu sync.RWMutex
	pf profilesFile
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{Dir: dir}
	if b, err := os.ReadFile(s.Path("profiles.json")); err == nil {
		_ = json.Unmarshal(b, &s.pf)
	}
	for i := range s.pf.Profiles {
		sc := &s.pf.Profiles[i].Schedule
		if _, err := ParseClock(sc.Time); err != nil {
			sc.Time = "03:00"
		}
	}
	if _, ok := s.find(s.pf.Active); !ok && len(s.pf.Profiles) > 0 {
		s.pf.Active = s.pf.Profiles[0].ID
	}
	return s, nil
}

// Path returns the path of a file inside the data directory.
func (s *Store) Path(name string) string { return filepath.Join(s.Dir, name) }

func (s *Store) find(id string) (Profile, bool) {
	for _, p := range s.pf.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

func (s *Store) save() error {
	b, _ := json.MarshalIndent(s.pf, "", "  ")
	return writeFileAtomic(s.Path("profiles.json"), b, 0o600)
}

func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Store) Profiles() []Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Profile(nil), s.pf.Profiles...)
}

func (s *Store) Profile(id string) (Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.find(id)
}

func (s *Store) ActiveID() string { s.mu.RLock(); defer s.mu.RUnlock(); return s.pf.Active }

// Active returns the selected profile; ok is false when none is configured.
func (s *Store) Active() (Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.find(s.pf.Active)
}

func (s *Store) SetActive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.find(id); !ok {
		return apperr.ErrNoProfile
	}
	s.pf.Active = id
	return s.save()
}

// SaveProfile creates (empty ID) or updates a profile and returns it. A new profile becomes the active one.
// An update keeps the schedule and, when p.Pass is empty, the saved password.
func (s *Store) SaveProfile(p Profile) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = newID()
		p.Schedule = defaultSchedule()
		s.pf.Profiles = append(s.pf.Profiles, p)
		s.pf.Active = p.ID
		return p, s.save()
	}
	for i, old := range s.pf.Profiles {
		if old.ID == p.ID {
			if p.Pass == "" {
				p.Pass = old.Pass
			}
			p.Schedule = old.Schedule
			s.pf.Profiles[i] = p
			return p, s.save()
		}
	}
	return p, apperr.ErrNoProfile
}

func (s *Store) DeleteProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.pf.Profiles[:0:0]
	found := false
	for _, p := range s.pf.Profiles {
		if p.ID == id {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		return apperr.ErrNoProfile
	}
	s.pf.Profiles = out
	if s.pf.Active == id {
		s.pf.Active = ""
		if len(out) > 0 {
			s.pf.Active = out[0].ID
		}
	}
	_ = os.Remove(s.Path("state-" + id + ".json"))
	return s.save()
}

// UpdateSchedule applies fn to a profile's schedule under lock and persists the result.
func (s *Store) UpdateSchedule(id string, fn func(*Schedule)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.pf.Profiles {
		if s.pf.Profiles[i].ID == id {
			fn(&s.pf.Profiles[i].Schedule)
			return s.save()
		}
	}
	return apperr.ErrNoProfile
}

// WriteFile atomically writes a file inside the data directory.
func (s *Store) WriteFile(name string, data []byte, perm os.FileMode) error {
	return writeFileAtomic(s.Path(name), data, perm)
}

// ReadFile reads a file from the data directory.
func (s *Store) ReadFile(name string) ([]byte, error) { return os.ReadFile(s.Path(name)) }
