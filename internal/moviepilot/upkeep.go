package moviepilot

import (
	"context"
	"net/http"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const (
	// The background jobs MoviePilot runs for every subscription; the
	// search job only exists while its scheduled search is enabled.
	jobRefresh = "subscribe_refresh"
	jobSearch  = "subscribe_search"
	jobRunning = "正在运行"
)

type scheduledJob struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	NextRun string `json:"next_run"`
}

// SubscriptionUpkeep reads the global subscription rule groups and the
// subscription jobs of MoviePilot's scheduler (its WebUI's 后台服务).
func (c *Client) SubscriptionUpkeep(ctx context.Context) (flow.SubscriptionUpkeep, error) {
	var groups struct {
		Value []string `json:"value"`
	}
	setting := call{method: http.MethodGet, path: "/api/v1/system/setting/SubscribeFilterRuleGroups"}
	if err := c.do(ctx, setting, &groups); err != nil {
		return flow.SubscriptionUpkeep{}, err
	}
	var jobs []scheduledJob
	if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/dashboard/schedule"}, &jobs); err != nil {
		return flow.SubscriptionUpkeep{}, err
	}
	return flow.SubscriptionUpkeep{
		FilterGroups: groups.Value, Refresh: findJob(jobs, jobRefresh), Search: findJob(jobs, jobSearch),
	}, nil
}

func findJob(jobs []scheduledJob, id string) *flow.Job {
	for _, j := range jobs {
		if j.ID == id {
			return &flow.Job{Running: j.Status == jobRunning, Next: j.NextRun}
		}
	}
	return nil
}
