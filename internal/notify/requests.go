package notify

import (
	"slices"
	"time"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const maxCompletedRequests = 50

// Completion history keeps identities and scope, never requester names,
// chat addresses, file samples, paths or provider diagnostics.
type completedRequest struct {
	Key         string    `json:"key"`
	Source      string    `json:"source"`
	MediaID     string    `json:"media_id"`
	Title       string    `json:"title"`
	Year        string    `json:"year,omitempty"`
	Season      *int      `json:"season,omitempty"`
	Start       int       `json:"start,omitempty"`
	Total       int       `json:"total,omitempty"`
	Download    bool      `json:"download,omitempty"`
	BestVersion bool      `json:"best_version,omitempty"`
	At          time.Time `json:"at"`
	Users       []int64   `json:"users"`
}

// Requests projects immutable state without reading MoviePilot, saving a
// file, or granting ownership from the API key's administrator identity.
func (n *Notifier) Requests(userID int64) []flow.RequestRecord {
	st := n.snapshot()
	var out []flow.RequestRecord
	for _, w := range st.Watches {
		if w.Source == "" || w.MediaID == "" || !requestedBy(w, userID) {
			continue
		}
		out = append(out, w.requestRecord())
	}
	for i := len(st.Completed) - 1; i >= 0; i-- {
		if slices.Contains(st.Completed[i].Users, userID) {
			out = append(out, st.Completed[i].record())
		}
	}
	return out
}

func requestedBy(w watch, userID int64) bool {
	return slices.ContainsFunc(w.Requesters, func(a flow.Actor) bool { return a.UserID == userID })
}

func (w watch) requestRecord() flow.RequestRecord {
	return flow.RequestRecord{
		Key: w.key(), SubscriptionID: w.SubscriptionID, Download: w.Download, IsDownload: w.Download != "",
		Target: flow.Target{Media: w.media(), Season: w.Season, StartEpisode: w.Start, BestVersion: w.BestVersion},
		Total:  w.Total, Delivered: slices.Clone(w.Delivered), MovieDelivered: w.MovieDelivered,
		Pending: w.ArrivedAt != nil, Unmapped: w.Unmapped,
	}
}

func (c completedRequest) record() flow.RequestRecord {
	w := watch{Source: c.Source, MediaID: c.MediaID, Title: c.Title, Year: c.Year, Season: c.Season}
	r := flow.RequestRecord{
		Key: "completed:" + c.Key, Total: c.Total, CompletedAt: c.At, IsDownload: c.Download,
		Target: flow.Target{Media: w.media(), Season: c.Season, StartEpisode: c.Start, BestVersion: c.BestVersion},
	}
	return r
}

// Record only newly settled, complete scopes. A torrent's guessed episode
// label or prior holdings cannot prove all of its own files have arrived.
func withCompleted(completed []completedRequest, w watch, now time.Time) []completedRequest {
	owned := w
	if owned.Download != "" {
		owned.Held = nil
	}
	if !owned.complete() || w.BestVersion || w.Unmapped || w.Source == "" || w.MediaID == "" {
		return completed
	}
	c := completedRequest{
		Key: w.key(), Source: w.Source, MediaID: w.MediaID, Title: w.Title, Year: w.Year,
		Season: w.Season, Start: w.Start, Total: w.Total, BestVersion: w.BestVersion,
		Download: w.Download != "", At: now,
	}
	for _, a := range w.Requesters {
		if !slices.Contains(c.Users, a.UserID) {
			c.Users = append(c.Users, a.UserID)
		}
	}
	next := slices.DeleteFunc(slices.Clone(completed), func(old completedRequest) bool { return old.Key == c.Key })
	next = append(next, c)
	return next[max(len(next)-maxCompletedRequests, 0):]
}
