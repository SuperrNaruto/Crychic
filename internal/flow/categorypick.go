package flow

import (
	"context"
	"fmt"
)

const (
	actionCategory     = "cg" // opens the category picker on a confirm card
	actionCategoryPick = "ch" // arg: opening generation and option, 0 = automatic

	labelCategory = "选择分类…"
	labelAuto     = "自动"
)

// categoryPicker is the option list one opening of the picker fixed. depth is
// the history length once the card it opened from sits on top of it, so a
// pick can tell that the card is still the screen below.
type categoryPicker struct {
	generation int
	depth      int
	options    []SubscriptionCategory
}

func categoryRow(sess session) []Button {
	return []Button{{Label: labelCategory, Data: data(sess.id, actionCategory, 0)}}
}

// chooseCategory applies the steps of the optional category choice.
func (e *Engine) chooseCategory(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionCategory:
		return e.openCategories(ctx, sess), true
	case actionCategoryPick:
		return e.pickCategory(sess, p.arg), true
	}
	return Reply{}, false
}

// openCategories lists the enabled categories of the target's kind. A failed
// read only flashes a notice: the card keeps its automatic classification.
func (e *Engine) openCategories(ctx context.Context, sess session) Reply {
	if sess.target == nil || sess.screen.state == nil {
		return Reply{Notice: msgInvalidChoice}
	}
	options, err := e.backend.SubscriptionCategories(ctx, sess.target.Media.Kind)
	if err != nil {
		e.log.Warn("subscription categories unavailable", "media", sess.target.Media.ID)
		return Reply{Notice: msgCategoryRead}
	}
	sess.editOptions++
	sess.categoryPick = &categoryPicker{generation: sess.editOptions, depth: min(len(sess.history)+1, maxHistory), options: options}
	return e.categoryOptions(sess)
}

const (
	msgCategoryRead     = "MoviePilot 的分类暂时读不到，自动分类照常使用哦～"
	msgCategoryOptional = "分类可选，不选时由 MoviePilot 按规则自动分类。"
)

func (e *Engine) categoryOptions(sess session) Reply {
	target, pick := *sess.target, sess.categoryPick
	icon := "🎬 "
	if target.Media.Kind == TV {
		icon = "📺 "
	}
	view := listView{
		heading: Heading(Plain(icon + "选择订阅分类")),
		image:   sess.picked.Media.PosterURL,
		note:    msgCategoryOptional,
		footer:  []Button{cancelButton(sess.id)},
		entries: []listEntry{{text: Lines(Line(Strong(targetName(target))), Line(Plain("当前 · "+categoryName(target))))}},
		menu:    [][]Button{{{Label: labelAuto, Data: categoryData(sess.id, pick, 0)}}},
	}
	for i, category := range pick.options {
		item := entry(i+1, Strong(subSetting(category.Name)))
		if target.CategoryID == category.ID {
			item.Tag = "当前"
		}
		view.entries = append(view.entries, listEntry{
			text:    Lines(item),
			buttons: []Button{{Label: fmt.Sprint(i + 1), Data: categoryData(sess.id, pick, i+1)}},
		})
	}
	return e.listPages(sess, view)
}

func categoryName(t Target) string {
	if t.CategoryName == "" {
		return labelAuto
	}
	return subSetting(t.CategoryName)
}

func categoryData(id uint64, pick *categoryPicker, option int) string {
	return data(id, actionCategoryPick, pick.generation*subEditOptionBase+option)
}

// pickCategory applies the choice to a copy of the target and shows the card
// the picker opened from again, in that card's place: the stale confirmation
// of the old card carries an old ticket and no longer writes.
func (e *Engine) pickCategory(sess session, arg int) Reply {
	pick := sess.categoryPick
	option := arg % subEditOptionBase
	if pick == nil || sess.target == nil || arg/subEditOptionBase != pick.generation ||
		option > len(pick.options) || len(sess.history) != pick.depth {
		return Reply{Notice: msgInvalidChoice}
	}
	target := *sess.target
	target.CategoryID, target.CategoryName = "", ""
	if option > 0 {
		target.CategoryID, target.CategoryName = pick.options[option-1].ID, pick.options[option-1].Name
	}
	sess.history = sess.history[:pick.depth-1]
	sess.categoryPick, sess.pages, sess.listing = nil, nil, ""
	sess.target = &target
	if !target.BestVersion {
		sess.focus = &target
	}
	e.store.put(sess)
	return categoryCard(sess, target)
}

// categoryCard is the confirm screen of target, drawn from the session alone
// so choosing a category repeats no read and starts no search.
func categoryCard(sess session, target Target) Reply {
	switch {
	case target.BestVersion:
		return upgradeCard(sess, target)
	case target.Season != nil && target.StartEpisode == 0:
		return startChoices(sess, *target.Season)
	}
	return confirmCard(sess, target)
}

// categoryFact is the confirm card's line naming the chosen category.
func categoryFact(t Target) Text {
	if t.CategoryName == "" {
		return nil
	}
	return Lines(Line(Strong("分类 "), Plain(subSetting(t.CategoryName))))
}
