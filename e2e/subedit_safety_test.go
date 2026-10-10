package e2e

import (
	"net/http"
	"strings"
	"testing"
)

func TestSubscriptionEditDraftsAndEmptyDefaults(t *testing.T) {
	h := subscriptionEditor(t)
	h.tap(alice, editMessage, "查看修改")
	assertEditCount(h, 0)
	chooseEditValue(h, "分辨率", "1")
	h.tap(alice, editMessage, "返回")
	h.tap(alice, editMessage, "放弃修改")
	h.shows(editMessage, "订阅详情")
	h.shows(editMessage, "1080p")
	h.tap(alice, editMessage, "编辑条件")
	h.tap(alice, editMessage, "规则组")
	h.tap(alice, editMessage, "沿用全局")
	h.tap(alice, editMessage, "完成选择")
	h.tap(alice, editMessage, "站点")
	h.tap(alice, editMessage, "默认站点")
	h.tap(alice, editMessage, "完成选择")
	h.tap(alice, editMessage, "查看修改")
	h.tap(alice, editMessage, "确认保存")
	assertEditPatch(h, map[string]any{"id": 1, "filter_groups": []string{}, "sites": []int{}})
	h.tap(alice, editMessage, "刷新")
	h.tap(alice, editMessage, "编辑条件")
	chooseEditValue(h, "分辨率", "1")
	h.tap(alice, editMessage, "取消")
	h.tap(alice, editMessage, "放弃修改")
	h.shows(editMessage, "已取消")
	assertEditCount(h, 1)
	h.tr.verify(t)
}

func TestSubscriptionEditRejectsChangedIdentityAndConditions(t *testing.T) {
	h := subscriptionEditor(t)
	chooseEditValue(h, "分辨率", "1")
	h.tap(alice, editMessage, "查看修改")
	confirm, _ := findButton(mustMessage(h, editMessage).rows, "确认保存")
	h.tap(bob, editMessage, "确认保存")
	for _, change := range []map[string]any{
		{"media_id": "999"}, {"media_source": "douban"}, {"season": 3},
		{"episode_group": "different-group"}, {"date": "2026-10-05 11:00:00"},
	} {
		sub := editableBreakingSeason()
		for key, value := range change {
			sub[key] = value
		}
		h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{sub})))
		h.tap(alice, editMessage, "确认保存")
		assertEditCount(h, 0)
	}
	h.mp.setRoute(subsPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, editMessage, "确认保存")
	assertEditCount(h, 0)
	sub := editableBreakingSeason()
	sub["resolution"] = "720p"
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{sub})))
	h.tap(alice, editMessage, "确认保存")
	h.shows(editMessage, "后台条件已变化")
	h.tapData(alice, editMessage, confirm)
	assertEditCount(h, 0)
	h.tap(alice, editMessage, "1")
	h.tap(alice, editMessage, "采用后台")
	h.tap(alice, editMessage, "完成处理")
	h.shows(editMessage, "720p")
	h.tr.verify(t)
}

func TestSubscriptionEditConfirmationCannotReplay(t *testing.T) {
	h := subscriptionEditor(t)
	chooseEditValue(h, "分辨率", "1")
	h.tap(alice, editMessage, "查看修改")
	old, _ := findButton(mustMessage(h, editMessage).rows, "确认保存")
	h.tap(alice, editMessage, "返回")
	chooseEditValue(h, "资源类型", "不限")
	h.tap(alice, editMessage, "查看修改")
	h.tapData(alice, editMessage, old)
	assertEditCount(h, 0)
	current, _ := findButton(mustMessage(h, editMessage).rows, "确认保存")
	h.tap(alice, editMessage, "确认保存")
	h.tapData(alice, editMessage, current)
	h.shows(editMessage, "订阅条件已保存")
	h.tap(alice, editMessage, "返回")
	h.shows(editMessage, "订阅清单")
	h.tap(alice, editMessage, "1")
	h.tap(alice, editMessage, "编辑条件")
	chooseEditValue(h, "分辨率", "不限")
	h.tap(alice, editMessage, "查看修改")
	stale, _ := findButton(mustMessage(h, editMessage).rows, "确认保存")
	h.restart()
	h.tapData(alice, editMessage, stale)
	h.shows(editMessage, "/subscribe")
	assertEditCount(h, 1)
	h.tr.verify(t)
}

func TestSubscriptionEditUnknownWriteOnlyRefreshes(t *testing.T) {
	h := subscriptionEditor(t)
	chooseEditValue(h, "分辨率", "1")
	h.tap(alice, editMessage, "查看修改")
	confirm, _ := findButton(mustMessage(h, editMessage).rows, "确认保存")
	h.mp.editResponse(subscriptionEditResponse{lost: true})
	h.tap(alice, editMessage, "确认保存")
	h.shows(editMessage, "保存结果待核对")
	h.tapData(alice, editMessage, confirm)
	h.tap(alice, editMessage, "刷新")
	h.shows(editMessage, "2160p / 4K")
	assertEditCount(h, 1)
	h.tr.verify(t)
}

func TestSubscriptionEditRefusalsAndRedirectNeverReplay(t *testing.T) {
	h := subscriptionEditor(t)
	responses := []subscriptionEditResponse{
		{status: http.StatusUnauthorized, body: `{"success":false,"message":"hidden-filter-token"}`},
		{status: http.StatusForbidden, body: `{"success":false,"message":"hidden-filter-token"}`},
		{status: http.StatusOK, body: `{"success":false,"message":"hidden-filter-token /srv/private","data":null}`},
		{status: http.StatusBadRequest, body: `{"success":false,"message":"hidden-filter-token /srv/private","data":null}`},
		{status: http.StatusRequestTimeout, body: `{"success":false,"message":"hidden-filter-token /srv/private","data":null}`},
		{status: http.StatusOK, body: `{}`},
		{location: h.mp.URL + "/api/v1/subscribe/"},
	}
	for i, response := range responses {
		if i > 0 {
			h.tap(alice, editMessage, "刷新")
			h.tap(alice, editMessage, "编辑条件")
		}
		chooseEditValue(h, "分辨率", "1")
		h.tap(alice, editMessage, "查看修改")
		h.mp.editResponse(response)
		h.tap(alice, editMessage, "确认保存")
		if response.status == http.StatusRequestTimeout || response.body == `{}` {
			h.shows(editMessage, "保存结果待核对")
		}
		assertEditCount(h, i+1)
	}
	if len(h.mp.subscriptionPatches()) != 0 {
		t.Fatal("a refused or redirected update was replayed")
	}
	assertNoEditSecrets(h)
	h.tr.verify(t)
}

func TestSubscriptionEditOptionsAndInputsStaySafe(t *testing.T) {
	h := subscriptionEditor(t)
	h.mp.setRoute(editGroupsPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, editMessage, "规则组")
	h.shows(editMessage, "编辑订阅条件")
	setEditDirectories(h)
	h.tap(alice, editMessage, "包含条件")
	const tooManyRunes = 257
	h.answer(alice, editMessage, strings.Repeat("长", tooManyRunes))
	h.shows(editMessage, "过长")
	h.answer(alice, editMessage, "line\nbreak")
	h.shows(editMessage, "单行")
	h.answer(alice, editMessage, "[Group]")
	h.tap(alice, editMessage, "起始集")
	h.answer(alice, editMessage, "0")
	h.shows(editMessage, "有效的集数")
	h.answer(alice, editMessage, "14")
	h.shows(editMessage, "有效的集数")
	h.answer(alice, editMessage, "第２集")
	h.tap(alice, editMessage, "规则组")
	h.tap(alice, editMessage, "2")
	h.tap(alice, editMessage, "完成选择")
	h.tap(alice, editMessage, "查看修改")
	h.mp.setRoute(editGroupsPath, ok(writeFixture(t, map[string]any{"rule_groups": []any{}})))
	h.tap(alice, editMessage, "确认保存")
	assertEditCount(h, 0)
	h.shows(editMessage, "规则组或站点已变化")
	setEditDirectories(h)
	h.tap(alice, editMessage, "规则组")
	h.tap(alice, editMessage, "完成选择")
	h.shows(editMessage, "[Group]")
	h.tap(alice, editMessage, "查看修改")
	h.tap(alice, editMessage, "确认保存")
	assertEditPatch(h, map[string]any{"id": 1, "include": `\[Group\]`, "start_episode": 2, "filter_groups": []string{"动画 & 经典", "高清优先"}})
	assertNoEditSecrets(h)
	h.tr.verify(t)
}
