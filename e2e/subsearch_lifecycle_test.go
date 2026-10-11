package e2e

import (
	"testing"
	"time"
)

func TestSubscriptionSearchFollowsWithoutReplayingWork(t *testing.T) {
	h := subscriptionSearchHarness(t)
	beginSubscriptionSearch(h)
	next := h.tg.expect("edit:2")
	h.mp.setSearchBatch(firstSubBatch, map[string]any{"state": "running", "phase": "searching"})
	h.wait(next, "the subscription batch reaching the site-search phase")
	h.shows(subSearchMessage, "搜索中")
	h.tap(bob, subSearchMessage, "停止本次搜索")
	next = h.tg.expect("edit:2")
	h.mp.setSearchBatch(firstSubBatch, map[string]any{"phase": "submitting"})
	h.wait(next, "the owner still following after a rejected callback")
	h.shows(subSearchMessage, "提交下载中")
	held := h.mp.holdCalls(subBatchRead, 1)[0]
	h.wait(held.entered, "an in-flight automatic subscription batch read")
	h.tap(alice, subSearchMessage, "停止本次搜索")
	h.wait(held.exited, "the owner's callback interrupting the automatic read")
	h.shows(subSearchMessage, "只请求停止本次搜索")
	h.tap(alice, subSearchMessage, "返回")
	h.tap(alice, subSearchMessage, "继续刷新")
	h.tap(alice, subSearchMessage, "停止刷新")
	before := h.mp.searchCalls(subBatchRead)
	const beyondSession = 11 * time.Minute
	h.advance(beyondSession)
	h.tap(alice, subSearchMessage, "刷新")
	h.shows(subSearchMessage, "/subscribe")
	if h.mp.searchCalls(subBatchRead) != before {
		t.Error("expired batch view resumed polling")
	}
	assertSubscriptionSearchCalls(h, 1, 0)
	h.tr.verify(t)
}

func TestSubscriptionSearchReceiptDeliveryOnlyRetriesTheReply(t *testing.T) {
	h := subscriptionSearchHarness(t)
	h.tap(alice, subSearchMessage, "立即搜索")
	confirm, _ := findButton(mustMessage(h, subSearchMessage).rows, "确认搜索")
	const retrySeconds = 1
	limited := h.tg.rejectNextEdit(subSearchMessage, retrySeconds)
	recovered := h.tg.expect("edit:2")
	h.tap(alice, subSearchMessage, "确认搜索")
	h.wait(limited, "rate limiting the successful search receipt")
	h.tapData(alice, subSearchMessage, confirm)
	h.wait(recovered, "delivery of the original search receipt")
	h.shows(subSearchMessage, "排队中")
	h.tap(alice, subSearchMessage, "返回")
	h.shows(subSearchMessage, "订阅详情")
	h.tapData(alice, subSearchMessage, confirm)
	h.tap(alice, subSearchMessage, "查看本次搜索")
	h.shows(subSearchMessage, "排队中")
	h.tap(alice, subSearchMessage, "关闭")
	assertSubscriptionSearchCalls(h, 1, 0)
	h.tr.verify(t)
}

func TestSubscriptionSearchTopicAndRestartDoNotGrantControl(t *testing.T) {
	h := subscriptionSearchHarness(t)
	h.tap(alice, subSearchMessage, "关闭")
	const requestMessage, otherMessage, reopenedMessage = 3, 4, 5
	const requestTopic, otherTopic = 17, 19
	h.sayInTopic(alice, requestTopic, "/subscribe")
	h.tap(alice, requestMessage, "1")
	h.tap(alice, requestMessage, "立即搜索")
	confirm, _ := findButton(mustMessage(h, requestMessage).rows, "确认搜索")
	h.tap(bob, requestMessage, "确认搜索")
	h.sayInTopic(alice, otherTopic, "/start")
	h.tapData(alice, otherMessage, confirm)
	assertSubscriptionSearchCalls(h, 0, 0)
	h.tap(alice, requestMessage, "确认搜索")
	h.showsInTopic(requestMessage, requestTopic)
	h.tap(alice, requestMessage, "停止本次搜索")
	stop, _ := findButton(mustMessage(h, requestMessage).rows, "确认停止搜索")
	h.restart()
	h.tapData(alice, requestMessage, stop)
	h.shows(requestMessage, "/subscribe")
	h.sayInTopic(alice, requestTopic, "/subscribe")
	h.tap(alice, reopenedMessage, "1")
	if _, offered := findButton(mustMessage(h, reopenedMessage).rows, "查看本次搜索"); offered {
		t.Error("restart reconstructed search ownership from a backend subscription")
	}
	h.showsInTopic(reopenedMessage, requestTopic)
	assertSubscriptionSearchCalls(h, 1, 0)
	h.tr.verify(t)
}

func TestSubscriptionSearchShutdownKeepsTheUnknownWriteConsumed(t *testing.T) {
	h := subscriptionSearchHarness(t)
	h.tap(alice, subSearchMessage, "立即搜索")
	confirm, _ := findButton(mustMessage(h, subSearchMessage).rows, "确认搜索")
	held := &heldCall{entered: make(chan struct{}), release: make(chan struct{}), exited: make(chan struct{})}
	t.Cleanup(held.allow)
	h.mp.searchResponse(subSearchPost, subscriptionSearchResponse{hold: held})
	h.press(alice, subSearchMessage, "确认搜索")
	h.wait(held.entered, "MoviePilot accepting the search before its response")
	h.restart()
	h.wait(held.exited, "the interrupted search response exiting")
	h.tapData(alice, subSearchMessage, confirm)
	h.shows(subSearchMessage, "/subscribe")
	assertSubscriptionSearchCalls(h, 1, 0)
	if h.mp.subscriptionBatch(firstSubBatch) == nil {
		t.Error("the unknown write did not actually create the external batch")
	}
	h.tr.verify(t)
}
