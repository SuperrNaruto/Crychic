package flow

import (
	"context"
	"fmt"
)

// msgCategoryApplies states what a category change reaches, on both the edit
// confirmation and the create picker.
const msgCategoryApplies = "新分类用于之后的下载与整理，已整理的文件保留在原目录。"

const msgSubEditCategoryRead = "MoviePilot 的分类暂时读不到，草稿还在，自动分类照常使用哦～"

// The category list is read when the field opens and stays fixed for that
// opening; choices carry the opening's generation like the other pickers.
func (e *Engine) readSubEditCategories(ctx context.Context, sess session) Reply {
	categories, err := e.backend.SubscriptionCategories(ctx, sess.subEdit.base.Kind)
	if err != nil {
		e.log.Warn("subscription categories unavailable", "subscription", sess.subEdit.base.ID)
		return Reply{Notice: msgSubEditCategoryRead}
	}
	sess.subEdit.categories = categories
	return e.subEditCategoryOptions(sess)
}

func (e *Engine) subEditCategoryOptions(sess session) Reply {
	edit := sess.subEdit
	view := listView{
		heading: Heading(Plain(msgSubEditTitle + " · " + editFieldName(edit.field))),
		note:    "这里只修改草稿，选择后返回编辑菜单。" + msgCategoryApplies,
		footer:  []Button{cancelButton(sess.id)},
		entries: []listEntry{{text: Sentence(edit.fieldStatus())}},
		menu: [][]Button{
			{{Label: "自动", Data: data(sess.id, actionSubEditClear, edit.options)}},
			subEditRestoreRow(sess),
		},
	}
	for i, category := range edit.categories {
		item := entry(i+1, Strong(subSetting(category.Name)))
		if edit.draft.CategoryID == category.ID {
			item.Tag = "当前"
		}
		view.entries = append(view.entries, listEntry{
			text:    Lines(item),
			buttons: []Button{{Label: fmt.Sprint(i + 1), Data: subEditChoiceData(sess, i+1)}},
		})
	}
	return e.listPages(sess, view)
}

func (e *Engine) pickSubEditCategory(sess session, index int) Reply {
	edit := sess.subEdit
	if index < 1 || index > len(edit.categories) {
		return Reply{Notice: msgInvalidChoice}
	}
	category := edit.categories[index-1]
	edit.draft.CategoryID, edit.draft.Category = category.ID, category.Name
	return e.subEditMenu(sess)
}
