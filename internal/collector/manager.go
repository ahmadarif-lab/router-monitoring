package collector

// Manager owns one Collector per profile but polls only the active one, so switching between a home
// and an office router costs nothing while the other router stays silent (and possibly unreachable).

import (
	"context"
	"sync"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/config"
)

type Manager struct {
	store *config.Store

	mu     sync.Mutex
	cols   map[string]*Collector
	ctx    context.Context
	cancel context.CancelFunc // stops the active collector
	cur    *Collector         // the collector currently running
	sugg   string             // profile that answers while the active one does not
}

func NewManager(s *config.Store) *Manager { return &Manager{store: s, cols: map[string]*Collector{}} }

// Collector returns the collector of a profile, creating it on first use.
func (m *Manager) Collector(id string) *Collector {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.collectorLocked(id)
}

func (m *Manager) collectorLocked(id string) *Collector {
	c, ok := m.cols[id]
	if !ok {
		c = NewCollector(m.store, id)
		m.cols[id] = c
	}
	return c
}

// Active returns the collector and profile that are selected; ok is false when no profile exists.
func (m *Manager) Active() (*Collector, config.Profile, bool) {
	p, ok := m.store.Active()
	if !ok {
		return nil, p, false
	}
	return m.Collector(p.ID), p, true
}

// Start begins polling the active profile and watching for a better candidate.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	m.runActive()
	go m.suggestLoop(ctx)
}

// runActive (re)starts polling for the currently selected profile.
func (m *Manager) runActive() {
	m.stopCurrent()
	p, ok := m.store.Active()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !ok || m.ctx == nil || m.ctx.Err() != nil {
		return
	}
	c := m.collectorLocked(p.ID)
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel, m.cur, m.sugg = cancel, c, ""
	go c.Run(ctx)
}

func (m *Manager) stopCurrent() {
	m.mu.Lock()
	cancel, cur := m.cancel, m.cur
	m.cancel, m.cur = nil, nil
	m.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	if d := cur.Done(); d != nil {
		select {
		case <-d:
		case <-time.After(20 * time.Second): // an in-flight poll finishes first
		}
	}
}

// Switch makes another profile the active one.
func (m *Manager) Switch(id string) error {
	if err := m.store.SetActive(id); err != nil {
		return err
	}
	m.runActive()
	return nil
}

// Forget drops the collector of a deleted profile.
func (m *Manager) Forget(id string) {
	wasActive := m.store.ActiveID() == id
	m.mu.Lock()
	delete(m.cols, id)
	m.mu.Unlock()
	if wasActive {
		m.runActive()
	}
}

// Stop halts polling and waits for the state to be saved.
func (m *Manager) Stop() { m.stopCurrent() }

// Suggestion is the id of another profile whose router answers while the active one is unreachable.
func (m *Manager) Suggestion() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sugg
}

func (m *Manager) suggestLoop(ctx context.Context) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		c, p, ok := m.Active()
		found := ""
		if ok {
			if err := c.View().Err; err != nil && err.Code == "unreachable" {
				for _, other := range m.store.Profiles() {
					if other.ID != p.ID && other.Valid() && m.Collector(other.ID).Alive() {
						found = other.ID
						break
					}
				}
			}
		}
		m.mu.Lock()
		m.sugg = found
		m.mu.Unlock()
	}
}
