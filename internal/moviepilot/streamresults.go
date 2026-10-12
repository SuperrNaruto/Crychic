package moviepilot

import (
	"encoding/json"
	"slices"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const (
	maxStreamResults      = 10000
	minimumReplaceBatches = 2
)

type searchEvent struct {
	Type         string           `json:"type"`
	Stage        string           `json:"stage"`
	Success      *bool            `json:"success"`
	Items        []torrentContext `json:"items"`
	ReplaceBatch bool             `json:"replace_batch"`
	BatchIndex   *int             `json:"batch_index"`
	BatchCount   *int             `json:"batch_count"`
	Total        *int             `json:"total_items"`
}

// Final replacements are accumulated privately, then published atomically
// with done. A partial replacement cannot authorize downloading any preview.
type streamResults struct {
	target    flow.Target
	preview   []flow.Torrent
	final     []flow.Torrent
	replaced  bool
	nextBatch int
	batches   int
	rawFinal  int
	expected  *int
}

func (s *streamResults) accept(raw []byte) (flow.TorrentSnapshot, bool, error) {
	var event searchEvent
	if json.Unmarshal(raw, &event) != nil || (event.Success != nil && !*event.Success) {
		return flow.TorrentSnapshot{}, false, errSSEProtocol
	}
	switch event.Type {
	case "heartbeat", "progress", "start":
		return flow.TorrentSnapshot{}, false, nil
	case "error":
		return flow.TorrentSnapshot{}, false, errSSEProtocol
	case "append", "replace":
		return s.update(event)
	case "done":
		return s.finish(event)
	default:
		return flow.TorrentSnapshot{}, false, errSSEProtocol
	}
}

func (s *streamResults) update(event searchEvent) (flow.TorrentSnapshot, bool, error) {
	if event.Type == "replace" || event.ReplaceBatch {
		return flow.TorrentSnapshot{}, false, s.replace(event)
	}
	if s.replaced || event.Items == nil {
		return flow.TorrentSnapshot{}, false, errSSEProtocol
	}
	items, err := projectStreamItems(event.Items, s.target, true)
	if err != nil {
		return flow.TorrentSnapshot{}, false, err
	}
	s.preview = mergeResources(s.preview, items)
	if len(s.preview) > maxStreamResults {
		return flow.TorrentSnapshot{}, false, errSSEProtocol
	}
	return flow.TorrentSnapshot{Items: slices.Clone(s.preview)}, len(items) > 0, nil
}

func (s *streamResults) replace(event searchEvent) error {
	if event.Items == nil || !s.validBatch(event) {
		return errSSEProtocol
	}
	items, err := projectStreamItems(event.Items, s.target, false)
	if err != nil {
		return err
	}
	s.final = mergeResources(s.final, items)
	s.rawFinal += len(event.Items)
	if s.rawFinal > maxStreamResults {
		return errSSEProtocol
	}
	return nil
}

func (s *streamResults) validBatch(event searchEvent) bool {
	if event.Type == "replace" {
		if !s.beginReplace(event) {
			return false
		}
		if !event.ReplaceBatch {
			return true
		}
	}
	if !s.expectedBatch(event) {
		return false
	}
	if event.Total != nil && (s.expected == nil || *event.Total != *s.expected) {
		return false
	}
	s.nextBatch++
	return true
}

func (s *streamResults) expectedBatch(event searchEvent) bool {
	if !event.ReplaceBatch || event.BatchIndex == nil || event.BatchCount == nil {
		return false
	}
	return *event.BatchIndex == s.nextBatch && *event.BatchCount == s.batches && s.nextBatch < s.batches
}

func (s *streamResults) beginReplace(event searchEvent) bool {
	if s.replaced {
		return false
	}
	s.replaced, s.expected = true, event.Total
	if !event.ReplaceBatch {
		return event.BatchIndex == nil && event.BatchCount == nil
	}
	if event.BatchCount == nil || *event.BatchCount < minimumReplaceBatches || *event.BatchCount > maxStreamResults {
		return false
	}
	s.batches = *event.BatchCount
	return true
}

func (s *streamResults) finish(event searchEvent) (flow.TorrentSnapshot, bool, error) {
	if !plainCompletion(event) || s.nextBatch != s.batches {
		return flow.TorrentSnapshot{}, false, errSSEProtocol
	}
	if !s.replaced {
		if event.Items == nil {
			return flow.TorrentSnapshot{}, false, errSSEProtocol
		}
		event.Type = "replace"
		if err := s.replace(event); err != nil {
			return flow.TorrentSnapshot{}, false, err
		}
	}
	if !s.totalsVerified(event) {
		return flow.TorrentSnapshot{}, false, errSSEProtocol
	}
	return flow.TorrentSnapshot{Items: slices.Clone(s.final), Final: true}, true, nil
}

// totalsVerified needs at least one reported total, and every reported total
// to match the final list; without one the result stays incomplete.
func (s *streamResults) totalsVerified(event searchEvent) bool {
	if s.expected == nil && event.Total == nil {
		return false
	}
	if s.expected != nil && *s.expected != s.rawFinal {
		return false
	}
	return event.Total == nil || *event.Total == s.rawFinal
}

func plainCompletion(event searchEvent) bool {
	return event.Stage == "done" && !event.ReplaceBatch && event.BatchIndex == nil && event.BatchCount == nil
}

func projectStreamItems(raw []torrentContext, target flow.Target, preview bool) ([]flow.Torrent, error) {
	var out []flow.Torrent
	for _, rc := range raw {
		item, ok := rc.toTorrent()
		if preview {
			item, ok = rc.previewTorrent()
		}
		if !ok {
			return nil, errSSEProtocol
		}
		if !preview && !torrentMatches(item, target) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func torrentMatches(item flow.Torrent, target flow.Target) bool {
	m, want := item.Media, target.Media
	if m.Source != want.Source || m.ID != want.ID || m.Kind != want.Kind {
		return false
	}
	return target.Season == nil || item.Season == nil || *item.Season == *target.Season
}

func mergeResources(previous, added []flow.Torrent) []flow.Torrent {
	out := slices.Clone(previous)
	indices := make(map[string]int, len(out))
	for i, item := range out {
		indices[item.Identity] = i
	}
	for _, item := range added {
		if i, ok := indices[item.Identity]; ok {
			out[i] = item
			continue
		}
		indices[item.Identity] = len(out)
		out = append(out, item)
	}
	return out
}
