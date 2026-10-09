package flow

import (
	"context"
	"fmt"
	"slices"
)

const (
	actionSearchSites = "ss" // reads searchable sites, never searches them
	actionSearchSite  = "st" // toggles one upstream site ID
	actionSearchAll   = "sa" // restores the default all-sites scope
	actionSearchStart = "sb" // explicitly starts searching the selected scope
	msgSearchSites    = "选择搜索站点"
)

func (e *Engine) chooseSearchSites(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionSearchSites:
		return e.searchSites(ctx, sess), true
	case actionSearchSite:
		if !slices.ContainsFunc(sess.sites, func(site SearchSite) bool { return site.ID == p.arg }) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		sess.searchSites = toggled(sess.searchSites, p.arg)
		return e.siteScope(sess), true
	case actionSearchAll:
		sess.searchSites = nil
		return e.siteScope(sess), true
	case actionSearchStart:
		return e.searching(sess, actionTorrentRun), true
	}
	return Reply{}, false
}

func (e *Engine) searchSites(ctx context.Context, sess session) Reply {
	if sess.focus == nil {
		return Reply{Notice: msgInvalidChoice}
	}
	sites, err := e.backend.SearchSites(ctx, sess.focus.Media.Kind)
	if err != nil {
		return e.notice("search sites", err)
	}
	if len(sites) == 0 {
		return Reply{Notice: "没有可搜索的站点，尚未开始搜索哦～"}
	}
	sess.sites, sess.searchSites = sites, nil
	return e.siteScope(sess)
}

func (e *Engine) siteScope(sess session) Reply {
	view := listView{
		heading: Heading(Plain("🔍 " + msgSearchSites)),
		note:    "先选择要访问的站点，再点开始搜索；这里不会重新筛选已有结果哦～",
		menu: [][]Button{
			{{Label: markShown(msgAllSites, len(sess.searchSites) == 0), Data: data(sess.id, actionSearchAll, 0)}},
			{{Label: "开始搜索", Data: data(sess.id, actionSearchStart, 0)}},
		},
		footer: []Button{cancelButton(sess.id)},
	}
	for i, site := range sess.sites {
		item := entry(i+1, Strong(site.Name))
		label := fmt.Sprint(i + 1)
		if slices.Contains(sess.searchSites, site.ID) {
			item.Tag, label = msgTickedTag, "✓ "+label
		}
		view.entries = append(view.entries, listEntry{
			text: Lines(item), buttons: []Button{{Label: label, Data: data(sess.id, actionSearchSite, site.ID)}},
		})
	}
	return e.listPages(sess, view)
}

func searchScopeRow(sess session) []Button {
	if sess.focus == nil {
		return nil
	}
	return []Button{{Label: msgSearchSites, Data: data(sess.id, actionSearchSites, 0)}}
}
