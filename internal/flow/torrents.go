package flow

import (
	"context"
	"fmt"
)

const (
	actionTorrents     = "ts" // searches the indexer sites for the card's target
	actionTorrentRun   = "tg" // runs that search (Follow data), leading on from the card
	actionTorrentRetry = "tr" // searches again after a failure
	actionTorrentAgain = "tq" // runs the repeated search, replacing the failure
	actionTorrentPick  = "tp" // arg: index into session torrents; shows the release
	actionTorrentGet   = "td" // arg: index; downloads the release

	torrentPageItems = 8

	msgTorrentsSearching = "🔍 正在各个站点搜索资源，要等一会儿哦…"
	msgTorrentsNote      = "按 MoviePilot 的优先级排好啦，点编号看详情～"
	msgTorrentAsk        = "要下载这个资源吗？"
	msgHitAndRun         = "⚠️ 这是 H&R 资源，下完要保种够时间，不然站点会记过哦。"
	msgDownloadUnknown   = "呜，没能确认有没有开始下载…先用 /tasks 看一眼，确认没在下再来找我哦。"
	msgDownloadLabel     = "确认下载"
)

// chooseTorrents applies the resource search steps; ok is false for others.
func (e *Engine) chooseTorrents(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionTorrents:
		return e.searching(sess, actionTorrentRun), true
	case actionTorrentRetry:
		return e.searching(sess, actionTorrentAgain), true
	case actionTorrentRun, actionTorrentAgain:
		return e.searchTorrents(ctx, sess), true
	case actionTorrentPick:
		return e.pickTorrent(sess, p.arg), true
	case actionTorrentGet:
		return e.download(ctx, sess, p.arg), true
	}
	return Reply{}, false
}

// torrentButton offers a resource search for the card's target.
func torrentButton(id uint64) Button {
	return Button{Label: "搜索资源", Data: data(id, actionTorrents, 0)}
}

// searching shows the card while the sites are searched: the search takes
// tens of seconds, so the platform runs it as the reply's next refresh
// rather than holding the button press.
func (e *Engine) searching(sess session, run string) Reply {
	if sess.focus == nil {
		return Reply{Notice: msgInvalidChoice}
	}
	reply := sess.picked.reply(Line(Plain(msgTorrentsSearching)), [][]Button{{cancelButton(sess.id)}})
	reply.Follow = data(sess.id, run, 0)
	return reply
}

// searchTorrents lists the releases found for the card's target; a failed
// search keeps the card and offers to search again.
func (e *Engine) searchTorrents(ctx context.Context, sess session) Reply {
	if sess.focus == nil {
		return Reply{Notice: msgInvalidChoice}
	}
	target := *sess.focus
	found, err := e.backend.SearchTorrents(ctx, target)
	if ctx.Err() != nil {
		// The owner pressed on, which decides what shows; a notice leaves
		// the screen and its 返回 history as they are.
		return Reply{Notice: msgCancelled}
	}
	sess.torrents, sess.release = found, nil
	e.store.put(sess)
	if err != nil {
		rows := [][]Button{{{Label: "重试", Data: data(sess.id, actionTorrentRetry, 0)}, cancelButton(sess.id)}}
		return sess.picked.replyLines(e.failure("torrent search", err).Text, rows)
	}
	if len(found) == 0 {
		text := Sentence(fmt.Sprintf("🔍 没搜到%s的资源 (｡•́︿•̀｡) 过段时间再来试试吧～", targetName(target)))
		rows := [][]Button{{{Label: "重试", Data: data(sess.id, actionTorrentRetry, 0)}, cancelButton(sess.id)}}
		return sess.picked.replyLines(text, rows)
	}
	return e.listPages(sess, torrentList(sess, target))
}

// pickTorrent shows one release under the card and asks to download it.
func (e *Engine) pickTorrent(sess session, index int) Reply {
	if index < 0 || index >= len(sess.torrents) {
		return Reply{Notice: msgInvalidChoice}
	}
	sess.release = &index
	e.store.put(sess)
	t := sess.torrents[index]
	text := torrentCard(t)
	if t.HitAndRun {
		text = append(text, Line(Plain(msgHitAndRun)))
	}
	text = append(text, Line(Strong(msgTorrentAsk)))
	rows := [][]Button{{{Label: msgDownloadLabel, Data: data(sess.id, actionTorrentGet, index)}, cancelButton(sess.id)}}
	return sess.picked.replyLines(text, rows)
}

// download adds release index to the downloader once. Steps of a session
// run one at a time and the release to confirm is cleared before the write,
// so a second tap only flashes a notice and the outcome stays on screen. A
// write is never offered again; an unknown result points to /tasks.
func (e *Engine) download(ctx context.Context, sess session, index int) Reply {
	if sess.release == nil || *sess.release != index || index >= len(sess.torrents) || sess.focus == nil {
		return Reply{Notice: msgInvalidChoice}
	}
	sess.release = nil
	e.store.put(sess)
	t := sess.torrents[index]
	id, err := e.backend.Download(ctx, t)
	if err != nil || id == "" {
		return sess.picked.replyLines(e.downloadFailure(t, id, err).Text, nil)
	}
	what := releaseName(*sess.focus, t)
	done := fmt.Sprintf("✅ 开始下载%s啦%s ヾ(≧▽≦*)o", what, e.noticeEnding(e.rememberDownload(ctx, sess, id, t)))
	return sess.picked.reply(Line(Plain(done)), nil)
}

// downloadFailure passes a MoviePilot refusal on; anything else may still
// have added the download.
func (e *Engine) downloadFailure(t Torrent, id string, err error) Reply {
	if _, safe := UserMessage(err); safe {
		return e.failure("download", err)
	}
	e.log.Error("download result unknown", "media", t.Media.ID, "id", id, "err", err)
	return Reply{Text: Sentence(msgDownloadUnknown)}
}

// rememberDownload registers the session owner for an arrival notice of
// download id, which brings the release's episodes of the card's target.
func (e *Engine) rememberDownload(ctx context.Context, sess session, id string, t Torrent) bool {
	target := *sess.focus
	req := Request{Download: id, Target: target, Requester: sess.owner}
	if season := target.Season; season != nil {
		req.Target.StartEpisode = 0
		req.SeasonEpisodes = sess.episodeCount(*season)
		if len(t.Episodes) > 0 {
			req.Target.StartEpisode, req.SeasonEpisodes = t.Episodes[0], t.Episodes[len(t.Episodes)-1]
		}
	}
	if err := e.watcher.Watch(ctx, req); err != nil {
		e.log.Error("cannot remember download for notification", "download", id, "err", err)
		return false
	}
	return true
}
