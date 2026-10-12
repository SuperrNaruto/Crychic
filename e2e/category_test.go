package e2e

import (
	"strings"
	"testing"
)

const (
	categoryPicker    = "选择订阅分类"
	msgPickSeasonText = "想订哪一季呀？"
)

// The category is an optional extra on the confirm card: choosing one
// replaces the card, so the old card's confirmation (older ticket) cannot
// subscribe, 返回 leads where the old card's did, and only the confirmation
// of the card that shows the category sends it.
func TestChooseCategoryWhenSubscribing(t *testing.T) {
	h := start(t, scenario{routes: conanRoutes()})
	h.say(alice, alice, "/search 名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	oldConfirm, _ := findButton(mustMessage(h, 1).rows, "从第 1 集开始")
	h.tap(alice, 1, "选择分类…")
	h.shows(1, categoryPicker)
	firstOpening, _ := findButton(mustMessage(h, 1).rows, "2")
	h.tap(alice, 1, "1")
	h.shows(1, "分类 </b>国漫")
	h.tapData(alice, 1, oldConfirm)
	h.tap(alice, 1, "返回")
	h.shows(1, msgPickSeasonText)
	h.tap(alice, 1, conanFirst)
	if strings.Contains(mustMessage(h, 1).text, "分类 </b>") {
		t.Fatal("a new pick must start with automatic classification")
	}
	h.tap(alice, 1, "选择分类…")
	h.tapData(alice, 1, firstOpening)
	h.shows(1, categoryPicker)
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "从第 1 集开始")
	h.shows(1, "帮你订好《名侦探柯南》第 1 季")
	if got := strings.Count(h.tr.String(), "-> MoviePilot "+subscribePath+"\n"); got != 1 {
		t.Fatalf("subscription writes = %d, want 1", got)
	}
	if !strings.Contains(h.tr.String(), `"media_category_id":"tv.dongman.cn"`) {
		t.Fatal("the chosen category never reached MoviePilot")
	}
	h.tr.verify(t)
}

// A subscription classified only by its stored path (no id) changes to a
// category: the update carries just the id, the saved row matches the draft
// by id, and the read-only check agrees.
func TestEditSubscriptionCategory(t *testing.T) {
	row := editableBreakingSeason()
	row["media_category_id"], row["media_category"] = nil, "国产剧"
	h := subscriptionEditorFor(t, row)
	h.shows(editMessage, "分类 </b>国产剧")
	h.tap(alice, editMessage, "分类")
	h.shows(editMessage, "当前 · 国产剧")
	h.tap(alice, editMessage, "1")
	h.shows(editMessage, "分类（已修改）")
	h.tap(alice, editMessage, "查看修改")
	h.shows(editMessage, "已整理的文件保留在原目录")
	h.tap(alice, editMessage, "确认保存")
	assertEditPatch(h, map[string]any{"id": 1, "media_category_id": "tv.dongman.cn"})
	h.shows(editMessage, "订阅条件已保存")
	h.tap(alice, editMessage, "核对结果")
	h.shows(editMessage, "已生效")
	h.tr.verify(t)
}
