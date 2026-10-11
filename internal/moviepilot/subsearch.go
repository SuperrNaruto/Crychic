package moviepilot

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const (
	subscriptionSearchPath = "/api/v1/subscribe/search/"
	subscriptionBatchPath  = "/api/v1/subscribe/execution/batches/"
	maxSearchBatchIDBytes  = 128
)

var (
	errSearchOutcome = errors.New("moviepilot: subscription search outcome unknown")
	errSearchRead    = errors.New("moviepilot: subscription execution response unavailable")
)

type searchSubmissionRecord struct {
	BatchID string   `json:"batch_id"`
	Batches []string `json:"batch_ids"`
	Target  *int     `json:"target_count"`
	Queued  *int     `json:"queued_count"`
	Ongoing *int     `json:"ongoing_count"`
	Single  *bool    `json:"single"`
}

func (c *Client) SearchSubscription(ctx context.Context, sub flow.Subscription) (flow.SubscriptionSearchReceipt, error) {
	if sub.ID <= 0 || (sub.Kind != flow.Movie && sub.Kind != flow.TV) {
		return flow.SubscriptionSearchReceipt{}, errSearchOutcome
	}
	var result searchSubmissionRecord
	cl := call{method: http.MethodPost, path: subscriptionSearchPath + strconv.Itoa(sub.ID), query: url.Values{"type": {kindType(sub.Kind)}}}
	if err := c.subscriptionExecutionCall(ctx, cl, &result); err != nil {
		return flow.SubscriptionSearchReceipt{}, err
	}
	return result.receipt()
}

func (r searchSubmissionRecord) receipt() (flow.SubscriptionSearchReceipt, error) {
	if r.Single == nil || !*r.Single || r.Target == nil || r.Queued == nil || r.Ongoing == nil {
		return flow.SubscriptionSearchReceipt{}, errSearchOutcome
	}
	if *r.Target != 1 {
		return flow.SubscriptionSearchReceipt{}, errSearchOutcome
	}
	if *r.Queued == 0 && *r.Ongoing == 1 {
		// An existing batch can include other subscriptions. Do not return its
		// identifier to flow, even though the superuser key could access it.
		return flow.SubscriptionSearchReceipt{Ongoing: true}, nil
	}
	if !r.newSingleBatch() {
		return flow.SubscriptionSearchReceipt{}, errSearchOutcome
	}
	return flow.SubscriptionSearchReceipt{BatchID: r.BatchID}, nil
}

func (r searchSubmissionRecord) newSingleBatch() bool {
	return *r.Queued == 1 && *r.Ongoing == 0 && len(r.Batches) == 1 &&
		r.Batches[0] == r.BatchID && validSearchBatchID(r.BatchID)
}

func validSearchBatchID(id string) bool {
	if id == "" || len(id) > maxSearchBatchIDBytes {
		return false
	}
	for _, ch := range id {
		if !searchBatchIDRune(ch) {
			return false
		}
	}
	return true
}

func searchBatchIDRune(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
}

func (c *Client) SubscriptionSearchBatch(ctx context.Context, id string) (flow.SubscriptionSearchBatch, error) {
	if !validSearchBatchID(id) {
		return flow.SubscriptionSearchBatch{}, errSearchRead
	}
	var result *subscriptionBatchRecord
	cl := call{method: http.MethodGet, path: subscriptionBatchPath + id}
	if err := c.subscriptionExecutionCall(ctx, cl, &result); err != nil {
		return flow.SubscriptionSearchBatch{}, err
	}
	if result == nil || !result.validCounts() || !result.consistentState() {
		return flow.SubscriptionSearchBatch{}, errSearchRead
	}
	return result.batch(), nil
}

func (c *Client) CancelSubscriptionSearch(ctx context.Context, id string) error {
	if !validSearchBatchID(id) {
		return errSearchOutcome
	}
	var result struct{}
	return c.subscriptionExecutionCall(ctx, call{method: http.MethodPut, path: subscriptionBatchPath + id + "/cancel"}, &result)
}

// All execution calls keep credentials at the configured origin and discard
// raw errors. POST and PUT are never retried by the application; missing
// success, redirects, partial bodies and 408 remain unknown write results.
func (c *Client) subscriptionExecutionCall(ctx context.Context, cl call, out any) error {
	client := *c
	client.http, cl.secret = c.stream, true
	err := client.do(ctx, cl, out)
	if err == nil || errors.Is(err, errAuth) {
		return err
	}
	if cl.method == http.MethodGet {
		return errSearchRead
	}
	if searchWriteRefused(err) {
		return &flow.UserError{Message: "MoviePilot 拒绝了这次订阅搜索操作，请到后台核对哦～"}
	}
	return errSearchOutcome
}

func searchWriteRefused(err error) bool {
	if _, safe := flow.UserMessage(err); safe {
		return true
	}
	var status *statusError
	return errors.As(err, &status) && rejectedSubscriptionStatus(status.code)
}
