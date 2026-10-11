package e2e

import (
	"fmt"
	"time"
)

// untilDigest is how far the clock moves from epoch (Monday noon) past the
// scenario's Monday 20:00 digest.
const untilDigest = 9 * time.Hour

// digestArrives moves the clock by elapsed and waits for the digest in
// both whitelisted users' private chats.
func (h *harness) digestArrives(elapsed time.Duration) {
	h.t.Helper()
	toAlice, toBob := h.tg.expect(fmt.Sprintf("send:%d", alice)), h.tg.expect(fmt.Sprintf("send:%d", bob))
	h.advance(elapsed)
	h.wait(toAlice, "the weekly digest to alice")
	h.wait(toBob, "the weekly digest to bob")
}

// Once a week every whitelisted user hears what reached the library since
// the last digest, asked for or not, each title and season once; a restart
// does not repeat a digest.
