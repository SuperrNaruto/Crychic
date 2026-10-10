package e2e

import (
	"net/http"
	"strings"
	"testing"
)

// The history boundary owns classification and privacy. Raw recovery_action
// text is intentionally hostile; only known structural fields may be shown.
func TestMyRequestsFailuresAreScoped(t *testing.T) {
	routes := myRequestRoutes(t)
	routes[failedTransfersPath] = ok(writeFixture(t, map[string]any{"list": requestFailureRecords(), "total": 9}))
	h := start(t, scenario{routes: routes})
	requestBreakingSeason(h)
	before := h.tr.String()
	h.say(alice, alice, "/requests")
	h.tap(alice, 2, "需留意")
	h.tap(alice, 2, "1")
	h.shows(2, "关联失败记录 2 条")
	h.shows(2, "目标存储访问")
	h.shows(2, "未知阶段")
	h.shows(2, "自动整理已暂停")
	h.shows(2, "自动重试已用尽")
	if strings.Contains(mustMessage(h, 2).text, "源文件访问") {
		t.Error("an unrelated or successful history row was attributed to this request")
	}
	h.tap(alice, 2, "刷新")
	sub := breakingSeason(1, 2)
	sub["episode_group"] = "unmapped-group"
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{sub})))
	h.tap(alice, 2, "刷新")
	h.shows(2, "剧集组映射未确认")
	h.shows(2, "<td>未知</td>")
	if strings.Contains(mustMessage(h, 2).text, "暂无任务记录") {
		t.Error("unmapped episode groups became a successful empty task read")
	}
	assertRequestReadsOnly(h, before)
	assertRequestPrivate(h)
	h.tr.verify(t)
}

func requestFailureRecords() []transfer {
	base := breakingBad.file("S02", "E03")
	base.ID, base.FailureStage = 1, "destination_access"
	base.AutoPaused, base.RetryExhausted = true, true
	base.Src, base.Error, base.RecoveryAction = "/private/"+requestPrivate, requestPrivate, requestPrivate
	unknown := base
	unknown.ID, unknown.FailureStage = 2, requestPrivate
	otherSource, otherKind, otherSeason, otherGroup, succeeded := base, base, base, base, base
	otherSource.ID, otherSource.MediaSource = 3, "douban"
	otherKind.ID, otherKind.Type = 4, "电影"
	otherSeason.ID, otherSeason.Seasons = 5, "S02-S03"
	otherGroup.ID, otherGroup.EpisodeGroup = 6, "different-episode-group"
	succeeded.ID, succeeded.Status = 7, true
	missing := base
	missing.ID, missing.MediaID = 8, ""
	rows := []transfer{base, unknown, otherSource, otherKind, otherSeason, otherGroup, succeeded, missing, base}
	for i := 2; i < len(rows)-1; i++ {
		rows[i].FailureStage = "source_access"
	}
	return rows
}

// A failed second page must preserve proven first-page issues, while
// independent download data remains usable. Closing a subscription never
// claims that the library has the requested files.
func TestMyRequestsKeepPartialReads(t *testing.T) {
	h := start(t, scenario{routes: myRequestRoutes(t)})
	requestBreakingSeason(h)
	installRequestProgress(h)
	seedRequestFailures(h, 50)
	down := route{status: http.StatusServiceUnavailable, fixture: "server_error.json"}
	for _, path := range []string{subsPath, queuePath, libraryShowPath, failedTransferPage(2)} {
		h.mp.setRoute(path, down)
	}
	before := h.tr.String()
	h.say(alice, alice, "/requests")
	h.shows(2, "失败历史未完整读取")
	h.tap(alice, 2, "需留意")
	h.tap(alice, 2, "1")
	h.shows(2, "下载中 37%")
	h.shows(2, "目标存储访问")
	h.shows(2, "媒体库暂时无法确认")
	h.shows(2, "订阅状态暂时无法确认")
	for _, path := range []string{subsPath, downloadsPath, queuePath} {
		h.mp.setRoute(path, ok("empty.json"))
	}
	replacement := breakingSeason(1, 3)
	replacement["name"], replacement["media_id"] = requestPrivate, "unrelated-media"
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{replacement})))
	h.mp.setRoute(libraryShowPath, ok(writeFixture(t, map[string][]int{})))
	h.mp.setRoute(failedTransfersPath, ok(writeFixture(t, map[string]any{"list": []transfer{}, "total": 0})))
	h.tap(alice, 2, "刷新")
	h.shows(2, "订阅已结束或已变更")
	h.shows(2, "未入库")
	h.shows(2, "关闭订阅不代表到货")
	assertRequestReadsOnly(h, before)
	assertRequestPrivate(h)
	h.tr.verify(t)
}

func TestMyRequestsBoundFailureHistory(t *testing.T) {
	h := start(t, scenario{routes: myRequestRoutes(t)})
	requestBreakingSeason(h)
	seedRequestFailures(h, 100)
	before := h.tr.String()
	h.say(alice, alice, "/requests")
	h.shows(2, "仅检查最近 100 条失败整理记录")
	reads := strings.Count(strings.TrimPrefix(h.tr.String(), before), "GET /api/v1/history/transfer?")
	if reads != 2 {
		t.Errorf("bounded failed history reads = %d, want 2", reads)
	}
	h.tap(alice, 2, "1")
	h.shows(2, "关联失败记录 1 条")
	assertRequestReadsOnly(h, before)
	h.tr.verify(t)
}

func seedRequestFailures(h *harness, unrelated int) {
	h.t.Helper()
	for range unrelated {
		h.mp.failedTransfer(duneFile())
	}
	file := breakingBad.file("S02", "E03")
	file.FailureStage, file.Error = "destination_access", requestPrivate
	h.mp.failedTransfer(file)
}

// A syntactically valid but incomplete envelope is not an empty history.
// Keeping this table at HTTP boundaries avoids tests of private parsers.
func TestMyRequestsRejectUnknownHistory(t *testing.T) {
	h := start(t, scenario{routes: myRequestRoutes(t)})
	requestBreakingSeason(h)
	before := h.tr.String()
	h.say(alice, alice, "/requests")
	for _, payload := range []any{nil, map[string]any{}, map[string]any{"list": []transfer{}}, map[string]any{"list": []transfer{}, "total": 5}} {
		h.mp.setRoute(failedTransfersPath, ok(writeFixture(t, payload)))
		h.tap(alice, 2, "刷新")
		h.shows(2, "失败历史未完整读取")
	}
	h.mp.setRoute(failedTransfersPath, ok("subscribe_rejected.json"))
	h.tap(alice, 2, "刷新")
	h.shows(2, "失败历史未完整读取")
	checkUnknownRequestSources(h)
	assertRequestReadsOnly(h, before)
	h.tr.verify(t)
}

func checkUnknownRequestSources(h *harness) {
	h.t.Helper()
	for _, source := range []struct{ path, label string }{
		{subsPath, "订阅状态"}, {downloadsPath, "下载状态"}, {queuePath, "整理状态"},
	} {
		h.mp.setRoute(source.path, ok(writeFixture(h.t, nil)))
		h.tap(alice, 2, "刷新")
		h.shows(2, source.label+"暂时无法确认")
	}
}
