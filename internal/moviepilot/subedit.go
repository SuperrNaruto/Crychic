package moviepilot

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// Public v3.1.1 updates use exclude_unset=True. Never copy a subscription
// response into a request: it also contains progress, paths and owner data.
func (c *Client) UpdateSubscription(ctx context.Context, update flow.SubscriptionUpdate) error {
	if update.ID <= 0 {
		return errors.New("moviepilot: invalid subscription update identity")
	}
	body := subscriptionPatch(update)
	if len(body) == 1 {
		return errors.New("moviepilot: empty subscription update")
	}
	client := *c
	client.http = c.stream // shared transport, no redirects or Cookie jar
	var result struct{}
	err := client.do(ctx, call{method: http.MethodPut, path: "/api/v1/subscribe/", body: body, secret: true}, &result)
	if err == nil || errors.Is(err, errAuth) {
		return err
	}
	if _, safe := flow.UserMessage(err); safe {
		return &flow.UserError{Message: "MoviePilot 拒绝了这次修改，请到后台核对订阅和条件哦～"}
	}
	var status *statusError
	if errors.As(err, &status) && rejectedWriteStatus(status.code) {
		return &flow.UserError{Message: "MoviePilot 拒绝了这次修改，请到后台核对订阅和条件哦～"}
	}
	return errors.New("moviepilot: subscription update outcome unknown")
}

func rejectedWriteStatus(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed,
		http.StatusConflict, http.StatusUnprocessableEntity, http.StatusTooManyRequests:
		return true
	}
	return false
}

func subscriptionPatch(update flow.SubscriptionUpdate) map[string]any {
	before, after := update.Before, update.After
	body := map[string]any{"id": update.ID}
	for _, field := range []struct{ name, before, after string }{
		{"quality", before.Quality, after.Quality}, {"resolution", before.Resolution, after.Resolution},
		{"effect", before.Effect, after.Effect}, {"include", before.Include, after.Include},
		{"exclude", before.Exclude, after.Exclude},
	} {
		if field.before != field.after {
			body[field.name] = clearedString(field.after)
		}
	}
	if !slices.Equal(before.FilterGroups, after.FilterGroups) {
		body["filter_groups"] = append([]string{}, after.FilterGroups...)
	}
	if !slices.Equal(before.Sites, after.Sites) {
		body["sites"] = append([]int{}, after.Sites...)
	}
	if before.StartEpisode != after.StartEpisode {
		body["start_episode"] = after.StartEpisode
	}
	if before.BestVersion != after.BestVersion {
		body["best_version"] = enabledNumber(after.BestVersion)
	}
	if before.BestVersionFull != after.BestVersionFull {
		body["best_version_full"] = enabledNumber(after.BestVersionFull)
	}
	return body
}

func clearedString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func enabledNumber(enabled bool) int {
	if enabled {
		return 1
	}
	return 0
}
