package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Rule names are mutable references. A changed custom rule must invalidate
// confirmation even if the group's own name and expression did not change.
func TestSubscriptionEditRuleCatalogAndDefinitions(t *testing.T) {
	h := subscriptionEditor(t)
	groups := []map[string]any{
		{"name": "自定义视频", "rule_string": "CUST1", "syntax_valid": true, "referenced_rules": []string{"CUST1"}},
		{"name": "损坏组", "rule_string": "(", "syntax_valid": false, "syntax_error": "hidden-filter-token"},
		{"name": "缺少引用", "rule_string": "MISSING", "syntax_valid": true, "referenced_rules": []string{"MISSING"}},
		{"name": "音乐专用", "media_type": "音乐", "rule_string": "1080P", "syntax_valid": true, "referenced_rules": []string{"1080P"}},
		{"name": "重名组", "rule_string": "1080P", "syntax_valid": true}, {"name": "重名组", "rule_string": "4K", "syntax_valid": true},
	}
	h.mp.setRoute(editGroupsPath, ok(writeFixture(t, map[string]any{"rule_groups": groups})))
	custom := func(expression string) route {
		return ok(writeFixture(t, map[string]any{"rules": []map[string]any{{"id": "CUST1", "include": expression}}}))
	}
	h.mp.setRoute(editCustomPath, custom("hidden-filter-token"))
	h.tap(alice, editMessage, "规则组")
	for _, name := range []string{"损坏组", "缺少引用", "音乐专用", "重名组"} {
		if strings.Contains(mustMessage(h, editMessage).text, name) {
			t.Errorf("unusable rule group %q was offered", name)
		}
	}
	h.tap(alice, editMessage, "沿用全局")
	h.tap(alice, editMessage, "1")
	h.tap(alice, editMessage, "完成选择")
	h.tap(alice, editMessage, "查看修改")
	h.mp.setRoute(editCustomPath, custom("changed-definition"))
	h.tap(alice, editMessage, "确认保存")
	h.shows(editMessage, "规则组或站点已变化")
	assertEditCount(h, 0)
	assertNoEditSecrets(h)
	h.tr.verify(t)
}

// Number buttons belong to a particular field and directory snapshot; a
// late resolution or reordered group choice must not choose a new target.
func TestSubscriptionEditChoiceIdentitySurvivesPaging(t *testing.T) {
	const count = 23
	h := subscriptionEditor(t)
	h.tap(alice, editMessage, "分辨率")
	stale, _ := findButton(mustMessage(h, editMessage).rows, "1")
	staleClear, _ := findButton(mustMessage(h, editMessage).rows, "不限")
	h.tap(alice, editMessage, "返回")
	h.tap(alice, editMessage, "资源类型")
	h.tapData(alice, editMessage, stale)
	h.tapData(alice, editMessage, staleClear)
	h.shows(editMessage, "· 资源类型")
	h.tap(alice, editMessage, "返回")
	groups := numberedEditGroups(count)
	h.mp.setRoute(editGroupsPath, ok(writeFixture(t, map[string]any{"rule_groups": groups})))
	h.tap(alice, editMessage, "规则组")
	stale, _ = findButton(mustMessage(h, editMessage).rows, "1")
	h.tap(alice, editMessage, "下一页 ›")
	h.tap(alice, editMessage, "23")
	h.shows(editMessage, "第 2/2 页")
	h.tap(alice, editMessage, "完成选择")
	groups[0], groups[1] = groups[1], groups[0]
	h.mp.setRoute(editGroupsPath, ok(writeFixture(t, map[string]any{"rule_groups": groups})))
	h.tap(alice, editMessage, "规则组")
	h.tapData(alice, editMessage, stale)
	h.tap(alice, editMessage, "完成选择")
	h.tap(alice, editMessage, "查看修改")
	h.tap(alice, editMessage, "确认保存")
	assertEditPatch(h, map[string]any{"id": 1, "filter_groups": []string{"动画 & 经典", "视频规则23"}})
	assertEditCount(h, 1)
	h.tr.verify(t)
}

func numberedEditGroups(count int) []map[string]any {
	var groups []map[string]any
	for n := 1; n <= count; n++ {
		name := fmt.Sprintf("视频规则%d", n)
		if n == 1 {
			name = "动画 & 经典"
		}
		groups = append(groups, map[string]any{"name": name, "rule_string": "1080P", "syntax_valid": true, "referenced_rules": []string{"1080P"}})
	}
	return groups
}

func TestSubscriptionEditDirectoryFailuresKeepDraft(t *testing.T) {
	h := subscriptionEditor(t)
	chooseEditValue(h, "资源类型", "不限")
	for _, path := range []string{editGroupsPath, editBuiltinPath, editCustomPath} {
		h.mp.setRoute(path, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
		h.tap(alice, editMessage, "规则组")
		h.shows(editMessage, "编辑订阅条件")
		setEditDirectories(h)
	}
	for _, path := range []string{editGroupsPath, editBuiltinPath, editCustomPath} {
		h.mp.setRoute(path, ok(writeFixture(t, nil)))
		h.tap(alice, editMessage, "规则组")
		h.shows(editMessage, "编辑订阅条件")
		setEditDirectories(h)
	}
	const sitesPath = "GET /api/v1/site/media/电视剧"
	h.mp.setRoute(sitesPath, route{status: http.StatusForbidden, fixture: "server_error.json"})
	h.tap(alice, editMessage, "站点")
	h.mp.setRoute(sitesPath, ok("empty.json"))
	h.tap(alice, editMessage, "站点")
	h.shows(editMessage, "没有可选项")
	h.tap(alice, editMessage, "返回")
	setEditDirectories(h)
	h.tap(alice, editMessage, "站点")
	h.tap(alice, editMessage, "2")
	h.tap(alice, editMessage, "完成选择")
	h.tap(alice, editMessage, "查看修改")
	h.mp.setRoute(sitesPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, editMessage, "确认保存")
	h.shows(editMessage, "确认修改")
	assertEditCount(h, 0)
	setEditDirectories(h)
	h.tap(alice, editMessage, "确认保存")
	assertEditPatch(h, map[string]any{"id": 1, "quality": nil, "sites": []int{11, 22}})
	h.tr.verify(t)
}
