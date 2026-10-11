package flow

import (
	"context"
	"fmt"
	"slices"
)

func (e *Engine) readSubEditChoices(ctx context.Context, sess session) Reply {
	edit := sess.subEdit
	if edit.field == editGroups {
		groups, err := e.backend.SubscriptionRules(ctx)
		if err != nil {
			return Reply{Notice: "暂时读不到完整规则组，草稿还在，没有保存修改哦～"}
		}
		edit.groups = usableSubGroups(groups, edit.base.Kind)
		if len(edit.groups) >= subEditOptionBase {
			return Reply{Notice: "规则组选项过多，请先到 MoviePilot 后台整理哦～"}
		}
	} else {
		sites, err := e.backend.SubscriptionSites(ctx, edit.base.Kind)
		if err != nil {
			return e.notice("subscription sites", err)
		}
		edit.sites = sites
		if len(edit.sites) >= subEditOptionBase {
			return Reply{Notice: "站点选项过多，请先到 MoviePilot 后台整理哦～"}
		}
	}
	edit.problem = ""
	return e.subEditDirectory(sess, "")
}

func usableSubGroups(groups []SubscriptionRuleGroup, kind Kind) []SubscriptionRuleGroup {
	var out []SubscriptionRuleGroup
	for _, group := range groups {
		if group.Available && (group.Kind == 0 || group.Kind == kind) {
			out = append(out, group)
		}
	}
	return out
}

func (e *Engine) subEditDirectory(sess session, holds string) Reply {
	edit := sess.subEdit
	view := listView{
		heading: Heading(Plain(msgSubEditTitle + " · " + editFieldName(edit.field))),
		note:    "选择已记入草稿，尚未保存；完成选择或返回都会保留勾选。最终确认后才写入 MoviePilot，选择期间只读取站点列表。",
		footer:  []Button{cancelButton(sess.id)},
	}
	reset := "沿用全局"
	if edit.field == editGroups {
		view.entries = edit.groupEntries(sess)
	} else {
		reset = "默认站点"
		view.entries = edit.siteEntries(sess)
	}
	if len(view.entries) == 0 {
		view.entries = []listEntry{{text: Sentence("当前没有可选项哦～可以保留原值或明确恢复默认设置。")}}
	}
	view.menu = [][]Button{
		{{Label: reset, Data: data(sess.id, actionSubEditClear, edit.options)}},
		subEditRestoreRow(sess),
		{{Label: "完成选择", Data: data(sess.id, actionSubEditApply, edit.options)}},
	}
	return e.listPageWith(sess, view, holds)
}

func (edit subscriptionEdit) groupEntries(sess session) []listEntry {
	var entries []listEntry
	for i, group := range edit.groups {
		facts := "适用两种媒体"
		if group.Kind != 0 {
			facts = group.Kind.String()
		}
		if group.Category != "" {
			facts += " · 分类 " + subSetting(group.Category)
		}
		entry := editDirectoryEntry(entry(i+1, Strong(subSetting(group.Name)), facts), i+1, slices.Contains(edit.draft.FilterGroups, group.Name))
		entry.buttons[0].Data = subEditChoiceData(sess, i+1)
		entries = append(entries, entry)
	}
	return entries
}

func (edit subscriptionEdit) siteEntries(sess session) []listEntry {
	var entries []listEntry
	for i, site := range edit.sites {
		entry := editDirectoryEntry(entry(i+1, Strong(subSetting(site.Name))), i+1, slices.Contains(edit.draft.Sites, site.ID))
		entry.buttons[0].Data = subEditChoiceData(sess, i+1)
		entries = append(entries, entry)
	}
	return entries
}

func editDirectoryEntry(item Block, number int, selected bool) listEntry {
	label := fmt.Sprint(number)
	if selected {
		item.Tag, label = msgTickedTag, "✓ "+label
	}
	return listEntry{text: Lines(item), buttons: []Button{{Label: label}}}
}

func (e *Engine) checkSubEditDirectories(ctx context.Context, edit subscriptionEdit) string {
	before := edit.base.conditions()
	if !slices.Equal(before.FilterGroups, edit.draft.FilterGroups) && len(edit.draft.FilterGroups) > 0 {
		groups, err := e.backend.SubscriptionRules(ctx)
		if err != nil {
			return msgSubEditDirectoryRead
		}
		if !edit.sameGroups(usableSubGroups(groups, edit.base.Kind)) {
			return msgSubEditDirectoryChanged
		}
	}
	if !slices.Equal(before.Sites, edit.draft.Sites) && len(edit.draft.Sites) > 0 {
		sites, err := e.backend.SubscriptionSites(ctx, edit.base.Kind)
		if err != nil {
			return msgSubEditDirectoryRead
		}
		if !edit.sameSites(sites) {
			return msgSubEditDirectoryChanged
		}
	}
	return ""
}

func (edit subscriptionEdit) sameGroups(current []SubscriptionRuleGroup) bool {
	for _, name := range edit.draft.FilterGroups {
		i := slices.IndexFunc(edit.groups, func(g SubscriptionRuleGroup) bool { return g.Name == name })
		j := slices.IndexFunc(current, func(g SubscriptionRuleGroup) bool { return g.Name == name })
		if i < 0 || j < 0 || edit.groups[i].Fingerprint != current[j].Fingerprint {
			return false
		}
	}
	return true
}

func (edit subscriptionEdit) sameSites(current []SearchSite) bool {
	for _, id := range edit.draft.Sites {
		i := slices.IndexFunc(edit.sites, func(s SearchSite) bool { return s.ID == id })
		j := slices.IndexFunc(current, func(s SearchSite) bool { return s.ID == id })
		if i < 0 || j < 0 || edit.sites[i] != current[j] {
			return false
		}
	}
	return true
}
