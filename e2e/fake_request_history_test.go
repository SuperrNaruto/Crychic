package e2e

import (
	"fmt"
	"net/http"
)

func failedTransferPage(page int) string {
	return fmt.Sprintf("%s&page=%d", failedTransfersPath, page)
}

// A diagnostic read is not a notifier poll. Its failures can be injected
// independently without advancing the full-poll completion barrier.
func (f *fakeMoviePilot) serveFailedTransferRoute(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Query().Get("status") != "false" {
		return false
	}
	f.mu.Lock()
	rt, found := f.routes[failedTransfersPath+"&page="+r.URL.Query().Get("page")]
	if !found {
		rt, found = f.routes[failedTransfersPath]
	}
	f.mu.Unlock()
	if found {
		writeEnvelope(w, rt.status, f.routeBody(r, rt))
	}
	return found
}
