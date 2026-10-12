package flow

import (
	"context"
	"fmt"
)

const (
	actionGroups    = "eg" // lists the picked show's episode groups
	actionGroupPick = "ep" // arg: index into the listed groups; offers its seasons

	labelGroups   = "剧集组…"
	msgNoGroups   = "这部剧没有可用的剧集组～"
	msgGroupEmpty = "这个剧集组没有可选的季～"
	// msgGroupLibrary stands in for library facts, which name standard seasons.
	msgGroupLibrary = "ℹ️ 按剧集组分季时，入库情况请到媒体库核对～"
)

// groupTypes name TMDB's seven episode group types, 1 to 7.
var groupTypes = map[int]string{1: "首播顺序", 2: "绝对顺序", 3: "DVD", 4: "数字发行", 5: "故事线", 6: "制作顺序", 7: "电视播出"}

// chooseGroups applies the episode group actions; ok is false for others.
func (e *Engine) chooseGroups(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionGroups:
		return e.groupList(ctx, sess), true
	case actionGroupPick:
		if p.arg < 0 || p.arg >= len(sess.groups) || sess.group.ID != "" {
			return Reply{Notice: msgInvalidChoice}, true
		}
		return e.pickGroup(ctx, sess, sess.groups[p.arg]), true
	}
	return Reply{}, false
}

// groupsOffered reports whether the season picker offers episode groups:
// only TMDB shows have them, and only in standard seasons.
func groupsOffered(sess session) bool {
	m := sess.picked.Media
	return m.Source == tmdbSource && m.Kind == TV && sess.group.ID == ""
}

// groupList lists the show's episode groups that number some episodes into
// some seasons; an empty group maps nothing to subscribe by. Nothing usable
// or a failed read only flashes a notice, keeping the standard seasons.
func (e *Engine) groupList(ctx context.Context, sess session) Reply {
	if !groupsOffered(sess) {
		return Reply{Notice: msgInvalidChoice}
	}
	all, err := e.backend.EpisodeGroups(ctx, sess.picked.Media)
	if err != nil {
		return e.notice("episode groups", err)
	}
	var groups []EpisodeGroup
	for _, g := range all {
		if g.Seasons > 0 && g.Episodes > 0 {
			groups = append(groups, g)
		}
	}
	if len(groups) == 0 {
		return Reply{Notice: msgNoGroups}
	}
	sess.groups = groups
	heading := fmt.Sprintf("🗂️ 《%s》的剧集组", sess.picked.Media.Title)
	view := listView{heading: Heading(Plain(heading)), footer: []Button{cancelButton(sess.id)}}
	for i, g := range groups {
		facts := []string{groupTypes[g.Type], fmt.Sprintf("%d 季", g.Seasons), fmt.Sprintf("%d 集", g.Episodes)}
		view.entries = append(view.entries, listEntry{
			text:    Lines(entry(i+1, Strong(truncate(g.Name, listTitleRunes)), facts...)),
			buttons: []Button{{Label: fmt.Sprint(i + 1), Data: data(sess.id, actionGroupPick, i)}},
		})
	}
	view.note = "按剧集组订阅时，季和集按该组的编排计算；入库和下载情况请到媒体库与 /tasks 核对。"
	return e.listPages(sess, view)
}

// pickGroup offers g's seasons to subscribe in its numbering. The library
// and downloads are known in standard seasons only, so they read as
// unknown here rather than as another season's.
func (e *Engine) pickGroup(ctx context.Context, sess session, g EpisodeGroup) Reply {
	read, err := e.backend.GroupSeasons(ctx, g.ID)
	if err != nil {
		return e.notice("group seasons", err)
	}
	var seasons []Season
	for _, s := range read {
		if s.EpisodeCount > 0 {
			seasons = append(seasons, s)
		}
	}
	if len(seasons) == 0 {
		return Reply{Notice: msgGroupEmpty}
	}
	sess.group, sess.library, sess.picked.LibraryUnknown = g, Library{}, true
	sess.download = false // resources are searched by standard seasons only
	sess.picked.LibraryNote = msgGroupLibrary
	return e.showSeasons(ctx, sess, seasonRead{seasons: seasons})
}
