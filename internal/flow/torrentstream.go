package flow

import (
	"bytes"
	"fmt"
	"slices"
)

const (
	torrentWaitingView = "torrent-waiting"
	torrentListView    = "torrent-list"
	torrentDetailView  = "torrent-detail"
	torrentFilterView  = "torrent-filter"
	torrentBatchView   = "torrent-batch"
	msgTorrentPartial  = "搜索尚未完成，当前是未经最终匹配的候选；可以浏览，暂不能下载哦～"
	msgTorrentChanged  = "这个资源已不在最终结果中或内容有变化，请返回列表重新选择哦～"
	msgTorrentRepeat   = "点「重新搜索资源」会再次访问站点，可能重复访问站点。"
)

func (e *Engine) prepareHunt(sess session) session {
	if sess.streaming && sess.hunt != nil {
		sess = syncedHunt(sess, sess.hunt.read())
	}
	sess.streaming = e.backend.StreamsTorrents()
	sess.hunts++
	if sess.streaming {
		// Reserve every previous number, but release its secret-bearing record.
		sess.torrents = make([]Torrent, max(sess.torrentIDs, len(sess.torrents)))
	} else {
		sess.torrents = nil
	}
	sess.torrentOrder, sess.release, sess.batch, sess.ticked = nil, nil, nil, nil
	sess.ticking, sess.missingOnly, sess.searchDone, sess.searchFinal = false, false, false, false
	sess.torrentSort, sess.torrentSite, sess.searchErr = 0, "", nil
	sess.searchView = torrentWaitingView
	return sess
}

func syncedHunt(sess session, progress huntProgress) session {
	base := sess.hunt.base
	items := make([]Torrent, base+len(progress.items))
	copy(items, sess.torrents)
	copy(items[base:], progress.items)
	sess.torrents = items
	sess.torrentOrder = progress.order
	sess.searchDone, sess.searchFinal, sess.searchErr = progress.done, progress.final, progress.err
	return sess
}

func (e *Engine) streamTorrentView(sess session, run string) Reply {
	if sess.searchView != torrentWaitingView && sess.searchView != torrentListView {
		return Reply{Notice: msgInvalidChoice}
	}
	sess = syncedHunt(sess, sess.hunt.read())
	if !sess.searchDone && len(sess.torrentOrder) == 0 {
		return searchingView(sess, run, e.now().Sub(sess.hunt.started))
	}
	return e.streamReleases(sess)
}

func (e *Engine) streamReleases(sess session) Reply {
	sess.searchView, sess.release = torrentListView, nil
	view := torrentList(sess, *sess.focus)
	view.source = torrentListView
	sess.pages = paged(sess.id, view)
	sess.listing = torrentListView
	sess.pageIndex = min(sess.pageIndex, len(sess.pages)-1)
	for i := range sess.pages {
		sess.pages[i] = streamFollowing(sess, sess.pages[i])
	}
	e.store.put(sess)
	return sess.pages[sess.pageIndex]
}

func streamFollowing(sess session, reply Reply) Reply {
	if !sess.searchDone && sess.hunt != nil {
		reply.Follow = data(sess.id, actionTorrentAgain, sess.hunt.token)
		reply.Wake = sess.hunt.done
	}
	return reply
}

func (e *Engine) refreshTorrent(sess session) Reply {
	if sess.hunt == nil || !sess.streaming {
		return Reply{Notice: msgInvalidChoice}
	}
	sess = syncedHunt(sess, sess.hunt.read())
	if sess.searchView == torrentDetailView && sess.release != nil {
		e.store.put(sess)
		return streamTorrentCard(sess, *sess.release)
	}
	return e.streamReleases(sess)
}

func streamTorrentCard(sess session, index int) Reply {
	t := sess.torrents[index]
	text := torrentCard(t)
	if t.HitAndRun {
		text = append(text, Line(Plain(msgHitAndRun)))
	}
	var rows [][]Button
	switch {
	case !sess.searchDone:
		text = append(text, Line(Plain(msgTorrentPartial)))
	case !sess.searchFinal || sess.searchErr != nil:
		text = append(text, Line(Plain(streamStatus(sess))))
	case t.Unverified || !slices.Contains(sess.torrentOrder, index):
		text = append(text, Line(Plain(msgTorrentChanged)))
	default:
		text = append(text, Line(Strong(msgTorrentAsk)))
		rows = append(rows, []Button{{Label: msgDownloadLabel, Data: data(sess.id, actionTorrentGet, index)}})
	}
	rows = append(rows, []Button{{Label: "刷新结果", Data: data(sess.id, actionTorrentRefresh, 0)}}, []Button{cancelButton(sess.id)})
	return sess.picked.replyLines(text, rows)
}

func streamStatus(sess session) string {
	if !sess.searchDone {
		return msgTorrentPartial
	}
	if sess.searchErr != nil || !sess.searchFinal {
		message, _ := UserMessage(sess.searchErr)
		return "⚠️ 结果不完整。" + message + " " + msgTorrentRepeat
	}
	if len(sess.torrentOrder) == 0 {
		return "没有匹配的资源哦～ " + msgTorrentRepeat
	}
	return "搜索完成，已按 MoviePilot 的优先级排列；编号保留原资源身份。" + msgTorrentRepeat
}

// Validate the exact displayed record against the latest completed snapshot.
// A preview never grants permission, even if a matching final event just arrived.
func (e *Engine) currentTorrent(sess session, index int) bool {
	if !sess.completedSearch() || sess.hunt == nil {
		return false
	}
	progress := sess.hunt.read()
	if !progress.complete() || !slices.Contains(progress.order, index) {
		return false
	}
	offset := index - sess.hunt.base
	if offset < 0 || offset >= len(progress.items) || sess.torrents[index].Unverified {
		return false
	}
	return bytes.Equal(sess.torrents[index].Record, progress.items[offset].Record)
}

func (sess session) completedSearch() bool {
	return sess.searchDone && sess.searchFinal && sess.searchErr == nil
}

func (p huntProgress) complete() bool { return p.done && p.final && p.err == nil }

func streamNote(sess session, total int) string {
	return fmt.Sprintf("共 %d 个，%s %s", total, streamStatus(sess), listHint(sess))
}
