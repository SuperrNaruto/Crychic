package e2e

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubscriptionEditReceiptAndPostWriteReadRecover(t *testing.T) {
	h := subscriptionEditor(t)
	chooseEditValue(h, "资源类型", "不限")
	h.tap(alice, editMessage, "查看修改")
	const retrySeconds = 1
	limited := h.tg.rejectNextEdit(editMessage, retrySeconds)
	recovered := h.tg.expect("edit:2")
	confirm, _ := findButton(mustMessage(h, editMessage).rows, "确认保存")
	h.tap(alice, editMessage, "确认保存")
	h.wait(limited, "the rate-limited subscription-edit receipt")
	h.tapData(alice, editMessage, confirm)
	h.wait(recovered, "the saved subscription-edit receipt")
	h.shows(editMessage, "订阅条件已保存")
	assertEditCount(h, 1)
	h.tap(alice, editMessage, "刷新")
	h.tap(alice, editMessage, "编辑条件")
	chooseEditValue(h, "画面格式", "不限")
	h.tap(alice, editMessage, "查看修改")
	h.mp.editResponse(subscriptionEditResponse{readFails: true})
	h.tap(alice, editMessage, "确认保存")
	h.shows(editMessage, "修改已受理，等待读取")
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{editableBreakingSeason()})))
	h.tap(alice, editMessage, "刷新")
	assertEditCount(h, 2)
	h.tr.verify(t)
}

func TestSubscriptionEditRedirectKeepsCredentialsAtOrigin(t *testing.T) {
	h := subscriptionEditor(t)
	var reached atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Store(true)
		writeEnvelope(w, http.StatusOK, `{"success":true,"data":null}`)
	}))
	defer other.Close()
	chooseEditValue(h, "资源类型", "不限")
	h.tap(alice, editMessage, "查看修改")
	h.mp.editResponse(subscriptionEditResponse{location: other.URL + "/api/v1/subscribe/"})
	h.tap(alice, editMessage, "确认保存")
	h.shows(editMessage, "保存结果待核对")
	if reached.Load() {
		t.Fatal("subscription update or API key followed a cross-origin redirect")
	}
	assertEditCount(h, 1)
	h.tr.verify(t)
}

func TestSubscriptionEditMovieKeepsEpisodeSettings(t *testing.T) {
	h := start(t, receiptScenario(t))
	h.say(alice, alice, "无海报影片")
	h.tap(alice, 1, "确认订阅")
	sub := map[string]any{"id": 1, "name": "无海报影片", "type": "电影", "media_source": "themoviedb", "media_id": "42", "best_version_full": 1}
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{sub})))
	h.say(alice, alice, "/subscribe")
	h.tap(alice, editMessage, "1")
	h.tap(alice, editMessage, "编辑条件")
	if _, found := findButton(mustMessage(h, editMessage).rows, "起始集"); found {
		t.Fatal("a movie offers an episode-range edit")
	}
	chooseEditValue(h, "洗版模式", "2")
	h.tap(alice, editMessage, "查看修改")
	h.tap(alice, editMessage, "确认保存")
	assertEditPatch(h, map[string]any{"id": 1, "best_version": 1})
	h.tap(alice, editMessage, "刷新")
	h.shows(editMessage, "洗版优先")
	h.tr.verify(t)
}

// A second live conversation may cancel the request while the old edit is
// still visible. Recreating the same row must not recreate ownership.
func TestSubscriptionEditRevokedRequestCannotSave(t *testing.T) {
	h := subscriptionEditor(t)
	chooseEditValue(h, "资源类型", "不限")
	h.tap(alice, editMessage, "查看修改")
	h.mp.setRoute(deleteFirst, ok("subscribe_deleted.json"))
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 3, "取消订阅")
	h.tap(alice, 3, "1")
	h.tap(alice, 3, "确认取消")
	h.tap(alice, editMessage, "确认保存")
	assertEditCount(h, 0)
	h.tr.verify(t)
}

func TestSubscriptionEditInputStaysInRequestTopic(t *testing.T) {
	const topic = 17
	h := subscriptionEditor(t)
	h.tap(alice, editMessage, "取消")
	h.sayInTopic(alice, topic, "/subscribe")
	h.tap(alice, 3, "1")
	h.tap(alice, 3, "编辑条件")
	h.tap(alice, 3, "包含条件")
	wrong := chatMessage{user: alice, chat: group, text: "wrong-topic", replyTo: 3, thread: topic + 1}
	h.tg.push(wrong.update(), "")
	h.answerQuoting(alice, 3, "中文字幕")
	h.tap(alice, 3, "查看修改")
	h.tap(alice, 3, "确认保存")
	h.showsInTopic(3, topic)
	assertEditPatch(h, map[string]any{"id": 1, "include": "中文字幕"})
	h.tr.verify(t)
}

// Unknown totals remain unknown; a later total change invalidates the
// confirmation rather than manufacturing acquired or missing episodes.
func TestSubscriptionEditStartRespectsCurrentTotal(t *testing.T) {
	h := subscriptionEditor(t)
	h.tap(alice, editMessage, "返回")
	sub := editableBreakingSeason()
	sub["total_episode"] = 0
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{sub})))
	h.tap(alice, editMessage, "编辑条件")
	h.tap(alice, editMessage, "起始集")
	h.answer(alice, editMessage, "20")
	h.tap(alice, editMessage, "查看修改")
	sub["total_episode"] = 13
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{sub})))
	h.tap(alice, editMessage, "确认保存")
	h.shows(editMessage, "后台条件已变化")
	assertEditCount(h, 0)
	h.tap(alice, editMessage, "完成处理")
	h.tap(alice, editMessage, "起始集")
	h.tap(alice, editMessage, "恢复原值")
	h.tap(alice, editMessage, "包含条件")
	const beyondSession = 11 * time.Minute
	h.advance(beyondSession)
	h.answerQuoting(alice, editMessage, "stale answer")
	h.shows(editMessage, "/subscribe")
	h.tr.verify(t)
}
