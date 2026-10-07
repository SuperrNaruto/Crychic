package flow

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"
)

const sessionIDBytes = 8

// session is one in-flight request conversation. It is treated as a value:
// every step stores a modified copy, never mutating a stored one.
type session struct {
	id          uint64
	owner       Actor
	menu        bool   // a home menu button is opening a feature; navigate clears it
	query       string // the current ordinary title search
	retry       recovery
	results     []Media
	picked      card
	seasons     []Season
	library     Library
	target      *Target
	focus       *Target      // what the card shows, for 搜索资源; nil before a target is chosen
	hunt        *torrentHunt // the resource search running for focus
	torrents    []Torrent
	torrentSort int     // index into torrentSorts
	torrentSite string  // the one site whose releases show; "" for all
	missingOnly bool    // only releases bringing missing episodes show
	release     *int    // index of the release shown to download
	ticking     bool    // the release list ticks releases to download together
	ticked      []int   // indices of the ticked releases, ascending
	batch       []int   // indices of the ticked releases asked to download
	chosen      []int   // seasons ticked in the multi-season picker
	chart       int     // index into charts being browsed
	page        int     // page of the chart last shown
	day         int     // calendar weekday shown, 1 Monday … 7 Sunday; 0 otherwise
	picks       []Media // the chart's picks
	noted       []bool  // picks already given a link and synopsis
	subs        []Subscription
	mine        []int // ids of subs the owner asked for
	kind        Kind  // the kind of subscription or history shown
	past        []PastSubscription
	tasks       []taskRef
	follow      following
	doomed      string // id of the download asked to be deleted
	pages       []Reply
	screen      screen   // what the conversation message shows now
	history     []screen // screens 返回 leads back to, latest last
	expires     time.Time
	gate        chan struct{} // serializes steps for this session, not unrelated users
}

// store keeps sessions in memory, keyed by the id embedded in button data,
// so one user can have several requests open at once.
type store struct {
	mu       sync.Mutex
	now      func() time.Time
	ttl      time.Duration
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
	var id uint64
	for id == 0 || s.sessions[id].id != 0 {
		var random [sessionIDBytes]byte
		_, _ = rand.Read(random[:])
		id = binary.LittleEndian.Uint64(random[:])
	}
	sess := session{id: id, owner: owner, results: results, expires: now.Add(s.ttl), gate: make(chan struct{}, 1)}
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
	current, ok := s.live(sess.id)
	if !ok {
		return
	}
	sess.expires = s.now().Add(s.ttl)
	sess.gate = current.gate
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
