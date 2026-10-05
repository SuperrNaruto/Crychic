package flow

import (
	"sync"
	"time"
)

// session is one in-flight request conversation. It is treated as a value:
// every step stores a modified copy, never mutating a stored one.
type session struct {
	id      uint64
	owner   Actor
	results []Media
	picked  card
	seasons []Season
	target  *Target
	expires time.Time
}

// store keeps sessions in memory, keyed by the id embedded in button data,
// so one user can have several requests open at once.
type store struct {
	mu       sync.Mutex
	now      func() time.Time
	ttl      time.Duration
	lastID   uint64
	sessions map[uint64]session
}

func newStore(now func() time.Time, ttl time.Duration) *store {
	return &store{now: now, ttl: ttl, sessions: map[uint64]session{}}
}

// create opens a session and sweeps expired ones.
func (s *store) create(owner Actor, results []Media) session {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, sess := range s.sessions {
		if now.After(sess.expires) {
			delete(s.sessions, id)
		}
	}
	s.lastID++
	sess := session{id: s.lastID, owner: owner, results: results, expires: now.Add(s.ttl)}
	s.sessions[sess.id] = sess
	return sess
}

func (s *store) get(id uint64) (session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.live(id)
}

// put replaces a still-live session and extends its lifetime.
func (s *store) put(sess session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.live(sess.id); !ok {
		return
	}
	sess.expires = s.now().Add(s.ttl)
	s.sessions[sess.id] = sess
}

// take removes and returns a session, so a step that ends the conversation
// (confirm, cancel) runs at most once even on a double tap.
func (s *store) take(id uint64) (session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.live(id)
	delete(s.sessions, id)
	return sess, ok
}

func (s *store) live(id uint64) (session, bool) {
	sess, ok := s.sessions[id]
	if !ok || s.now().After(sess.expires) {
		return session{}, false
	}
	return sess, true
}
