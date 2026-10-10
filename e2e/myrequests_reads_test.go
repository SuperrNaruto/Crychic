package e2e

import (
	"testing"
	"time"
)

// One slow source cannot spend the shared budget before the others even
// start. Real HTTP admission barriers prove independent reads, and only
// the slow subscription is left held until the production deadline.
func TestMyRequestsReadSourcesIndependently(t *testing.T) {
	h := start(t, scenario{routes: myRequestRoutes(t)})
	requestBreakingSeason(h)
	installRequestProgress(h)
	h.say(alice, alice, "/requests")
	h.tap(alice, 2, "1")
	before := h.tr.String()
	const heldReadCount = 1
	sub := h.mp.holdCalls(subsPath, heldReadCount)[0]
	download := h.mp.holdCalls(downloadsPath, heldReadCount)[0]
	library := h.mp.holdCalls(libraryShowPath, heldReadCount)[0]
	done := h.press(alice, 2, "刷新")
	h.wait(sub.entered, "the held subscription read")
	h.wait(download.entered, "the independent download read")
	h.wait(library.entered, "the independent media library read")
	download.allow()
	library.allow()
	const requestReplyBudget = 2 * actionTimeout
	select {
	case <-done:
	case <-time.After(requestReplyBudget):
		t.Fatal("the read-only request detail exceeded its shared budget")
	}
	h.wait(sub.exited, "the slow subscription read being cancelled")
	h.shows(2, "订阅状态暂时无法确认")
	h.shows(2, "已入库")
	h.shows(2, "下载中 37%")
	h.shows(2, "整理中")
	assertRequestReadsOnly(h, before)
	h.tr.verify(t)
}
