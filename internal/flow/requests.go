package flow

import (
	"context"
	"fmt"
	"slices"
)

const (
	actionRequests       = "rq"
	actionRequestPick    = "rp"
	actionRequestRefresh = "rr"
	requestListSource    = "requests"
)

const (
	requestsTracking = iota
	requestsAttention
	requestsCompleted
)

var requestModes = []string{"跟踪中", "需留意", "最近完成"}

func (e *Engine) Requests(ctx context.Context, actor Actor) Reply {
	sess := e.store.create(actor, nil)
	return e.shown(sess.id, e.listRequests(ctx, sess, requestsTracking))
}

func (e *Engine) chooseRequests(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionRequests:
		if p.arg < requestsTracking || p.arg > requestsCompleted {
			return Reply{Notice: msgInvalidChoice}, true
		}
		return e.listRequests(ctx, sess, p.arg), true
	case actionRequestPick, actionRequestRefresh:
		return e.requestDetail(ctx, sess, p.arg), true
	}
	return Reply{}, false
}

func requestAction(action string) bool {
	return action == actionRequests || action == actionRequestPick || action == actionRequestRefresh
}

func (e *Engine) listRequests(ctx context.Context, sess session, mode int) Reply {
	all := e.watcher.Requests(sess.owner.UserID)
	if sess.menu && len(all) == 0 {
		return Reply{Notice: "还没有通过我登记的请求哦～"}
	}
	records := slices.DeleteFunc(slices.Clone(all), func(r RequestRecord) bool {
		return r.CompletedAt.IsZero() == (mode == requestsCompleted)
	})
	var read requestSources
	if len(records) > 0 && mode != requestsCompleted {
		read = e.readRequestSources(ctx, records)
	}
	view := listView{
		heading: Heading(Plain("📋 我的请求 · " + requestModes[mode])), source: requestListSource,
		menu: requestMenu(sess.id, mode), footer: browseRow(sess.id), note: read.note(mode),
	}
	for _, record := range records {
		d := read.observe(record)
		if mode == requestsAttention && !d.attention() {
			continue
		}
		index := sess.requestIndex(record.Key)
		view.entries = append(view.entries, d.entry(sess.id, index))
	}
	if len(view.entries) == 0 {
		view.entries = []listEntry{{text: Sentence(emptyRequests(mode, len(all)))}}
	}
	return e.listPages(sess, view)
}

func (sess *session) requestIndex(key string) int {
	if index := slices.Index(sess.requestKeys, key); index >= 0 {
		return index
	}
	sess.requestKeys = append(slices.Clip(sess.requestKeys), key)
	return len(sess.requestKeys) - 1
}

func requestMenu(id uint64, mode int) [][]Button {
	var switches []Button
	for i, name := range requestModes {
		switches = append(switches, Button{Label: markShown(name, i == mode), Data: data(id, actionRequests, i)})
	}
	return [][]Button{switches, {{Label: "刷新", Data: data(id, actionRequests, mode)}}}
}

func emptyRequests(mode, total int) string {
	if total == 0 && mode != requestsCompleted {
		return "还没有通过我登记的请求哦～旧的全局任务不会自动算作你的请求。"
	}
	switch mode {
	case requestsAttention:
		return "本次读取没有发现需留意的请求；未匹配到记录不代表一定没有异常哦～"
	case requestsCompleted:
		return "还没有完成记录哦～只记录本功能启用后确认到货的完整请求范围。"
	}
	return "目前没有正在跟踪的请求，可以查看最近完成哦～"
}

func (e *Engine) ownedRequest(sess session, index int) (RequestRecord, bool) {
	if index < 0 || index >= len(sess.requestKeys) {
		return RequestRecord{}, false
	}
	for _, r := range e.watcher.Requests(sess.owner.UserID) {
		if r.Key == sess.requestKeys[index] {
			return r, true
		}
	}
	return RequestRecord{}, false
}

func (e *Engine) requestDetail(ctx context.Context, sess session, index int) Reply {
	record, owned := e.ownedRequest(sess, index)
	if !owned {
		return Reply{Notice: "这条请求已不在你的记录中，请返回后刷新哦～"}
	}
	detail := e.readRequestDetail(ctx, record)
	if _, stillOwned := e.ownedRequest(sess, index); !stillOwned {
		return Reply{Notice: "这条请求已不在你的记录中，请返回后刷新哦～"}
	}
	view := detail.view()
	view.menu = [][]Button{{{Label: "刷新", Data: data(sess.id, actionRequestRefresh, index)}}}
	view.footer = browseRow(sess.id)
	return e.listPages(sess, view)
}

func (d requestDetail) entry(id uint64, index int) listEntry {
	name := taskName(truncate(d.record.Target.Media.Title, listTitleRunes), d.record.Target.Season)
	return listEntry{
		text:    Lines(entry(index+1, Strong(name), d.summary())),
		buttons: []Button{{Label: fmt.Sprint(index + 1), Data: data(id, actionRequestPick, index)}},
	}
}
