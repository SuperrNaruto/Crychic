package e2e

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const editMessage = 2

func editableBreakingSeason() map[string]any {
	sub := breakingSeason(1, 2)
	sub["quality"], sub["resolution"], sub["effect"] = "WEB-DL", "1080p", "HDR"
	sub["include"], sub["exclude"] = "old-private-expression", "old-exclusion"
	sub["sites"], sub["filter_groups"] = []int{11}, []string{"动画 & 经典"}
	sub["search_interval"], sub["note"], sub["lack_episode"] = 24, []int{2, 4}, 11
	sub["current_priority"], sub["episode_priority"] = 70, map[string]int{"2": 70}
	sub["downloader"], sub["save_path"] = "private-downloader", "/srv/private/subscription"
	return sub
}

func subscriptionEditor(t *testing.T) *harness {
	t.Helper()
	return subscriptionEditorFor(t, editableBreakingSeason())
}

func subscriptionEditorFor(t *testing.T, row map[string]any) *harness {
	t.Helper()
	h := start(t, scenario{quiet: "0s", routes: map[string]route{
		searchPath: ok("search_breaking_bad.json"), breakDetails: ok("detail_breaking_bad.json"),
		seasonsPath: ok("seasons_breaking_bad.json"), breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
		subsPath:      ok(writeFixture(t, []map[string]any{row})),
	}})
	h.say(alice, alice, "/search 绝命毒师")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 2 季")
	h.tap(alice, 1, "从第 1 集开始")
	setEditDirectories(h)
	h.say(alice, alice, "/subscribe")
	h.tap(alice, editMessage, "1")
	h.tap(alice, editMessage, "编辑条件")
	return h
}

func setEditDirectories(h *harness) {
	h.t.Helper()
	groups := []map[string]any{
		{"name": "动画 & 经典", "rule_string": "1080P", "media_type": "电视剧", "syntax_valid": true, "referenced_rules": []string{"1080P"}},
		{"name": "高清优先", "rule_string": "4K > 1080P", "syntax_valid": true, "referenced_rules": []string{"4K", "1080P"}},
	}
	h.mp.setRoute(editGroupsPath, ok(writeFixture(h.t, map[string]any{"count": len(groups), "rule_groups": groups})))
	h.mp.setRoute(editBuiltinPath, ok(writeFixture(h.t, map[string]any{"rules": []map[string]any{
		{"id": "1080P", "include": []string{"1080[pi]|x1080"}}, {"id": "4K", "include": []string{"4k|2160p|x2160"}},
	}})))
	h.mp.setRoute(editCustomPath, ok(writeFixture(h.t, map[string]any{"rules": []any{}})))
	h.mp.setRoute("GET /api/v1/site/media/电视剧", ok(writeFixture(h.t, []map[string]any{
		{"id": 11, "name": "站点甲"}, {"id": 22, "name": "站点乙"},
	})))
}

func chooseEditValue(h *harness, field, choice string) {
	h.t.Helper()
	h.tap(alice, editMessage, field)
	h.tap(alice, editMessage, choice)
}

func TestSubscriptionEditsSaveOnlyConfirmedChanges(t *testing.T) {
	h := subscriptionEditor(t)
	chooseEditValue(h, "分辨率", "1")
	chooseEditValue(h, "资源类型", "不限")
	chooseEditValue(h, "画面格式", "不限")
	h.tap(alice, editMessage, "包含条件")
	h.answer(alice, editMessage, "Group.+|简体")
	chooseEditValue(h, "排除条件", "不限")
	h.tap(alice, editMessage, "规则组")
	h.tap(alice, editMessage, "✓ 1")
	h.tap(alice, editMessage, "2")
	h.tap(alice, editMessage, "完成选择")
	h.tap(alice, editMessage, "站点")
	h.tap(alice, editMessage, "✓ 1")
	h.tap(alice, editMessage, "2")
	h.tap(alice, editMessage, "完成选择")
	h.tap(alice, editMessage, "起始集")
	h.answer(alice, editMessage, "第３集")
	chooseEditValue(h, "洗版模式", "3")
	h.tap(alice, editMessage, "查看修改")
	h.shows(editMessage, "确认修改")
	assertEditCount(h, 0)
	concurrent := editableBreakingSeason()
	concurrent["search_interval"], concurrent["current_priority"], concurrent["lack_episode"] = 48, 80, 10
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{concurrent})))
	h.tap(alice, editMessage, "确认保存")
	h.shows(editMessage, "订阅条件已保存")
	assertEditPatch(h, map[string]any{
		"id": 1, "resolution": "4k|2160p|x2160", "quality": nil, "effect": nil,
		"include": `Group\.\+|简体`, "exclude": nil, "filter_groups": []string{"高清优先"},
		"sites": []int{22}, "start_episode": 3, "best_version": 1, "best_version_full": 1,
	})
	saved := h.mp.editableSubscription(1)
	if saved["lack_episode"] != float64(10) || saved["current_priority"] != float64(80) || saved["search_interval"] != float64(48) {
		t.Fatal("editing conditions overwrote runtime facts or untouched backend settings")
	}
	h.tap(alice, editMessage, "刷新")
	h.shows(editMessage, "从第 3 集开始")
	h.tap(alice, editMessage, "返回")
	h.shows(editMessage, "订阅清单")
	assertNoEditSecrets(h)
	h.tr.verify(t)
}

func assertEditCount(h *harness, expected int) {
	h.t.Helper()
	count := strings.Count(h.tr.String(), "-> MoviePilot "+editSubscriptionPath+"\n")
	if count != expected {
		h.t.Fatalf("subscription updates = %d, want %d", count, expected)
	}
}

func assertEditPatch(h *harness, want map[string]any) {
	h.t.Helper()
	patches := h.mp.subscriptionPatches()
	if len(patches) == 0 {
		h.t.Fatal("subscription update never reached MoviePilot")
	}
	raw, _ := json.Marshal(want)
	var normalized map[string]any
	_ = json.Unmarshal(raw, &normalized)
	if !reflect.DeepEqual(patches[len(patches)-1], normalized) {
		h.t.Fatalf("update fields = %#v, want %#v", patches[len(patches)-1], normalized)
	}
}

func assertNoEditSecrets(h *harness) {
	h.t.Helper()
	for _, private := range []string{"old-private-expression", "old-exclusion", "private-downloader", "/srv/private", "hidden-filter-token"} {
		if strings.Contains(h.tr.String(), private) {
			h.t.Errorf("subscription editing disclosed %q", private)
		}
	}
}
