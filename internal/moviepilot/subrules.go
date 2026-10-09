package moviepilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

type subscriptionRuleRecord struct {
	Name       string   `json:"name"`
	Expression string   `json:"rule_string"`
	Type       string   `json:"media_type"`
	Category   string   `json:"category"`
	Valid      bool     `json:"syntax_valid"`
	Rules      []string `json:"referenced_rules"`
}

func (c *Client) SubscriptionRules(ctx context.Context) ([]flow.SubscriptionRuleGroup, error) {
	var groups struct {
		Groups []subscriptionRuleRecord `json:"rule_groups"`
	}
	query := url.Values{"include_usage": {"false"}}
	err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/rule/groups", query: query, secret: true}, &groups)
	if err != nil || groups.Groups == nil {
		return nil, errors.New("moviepilot: subscription rule groups unavailable")
	}
	rules, err := c.subscriptionRuleDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, group := range groups.Groups {
		counts[group.Name]++
	}
	var out []flow.SubscriptionRuleGroup
	for _, group := range groups.Groups {
		if group.Name == "" || counts[group.Name] != 1 {
			continue
		}
		out = append(out, group.project(rules))
	}
	return out, nil
}

// Definitions are hashed in memory. Custom filters and usage records may
// contain private text, so only completeness and a digest leave this package.
func (c *Client) subscriptionRuleDefinitions(ctx context.Context) (map[string]string, error) {
	rules := map[string]string{}
	for _, path := range []string{"/api/v1/rule/builtin", "/api/v1/rule/custom"} {
		var body struct {
			Rules []json.RawMessage `json:"rules"`
		}
		query := url.Values{}
		if path == "/api/v1/rule/custom" {
			query.Set("include_group_refs", "false")
		}
		err := c.do(ctx, call{method: http.MethodGet, path: path, query: query, secret: true}, &body)
		if err != nil || body.Rules == nil {
			return nil, errors.New("moviepilot: subscription rule definitions unavailable")
		}
		for _, raw := range body.Rules {
			indexRuleDefinition(rules, raw)
		}
	}
	return rules, nil
}

func indexRuleDefinition(rules map[string]string, raw json.RawMessage) {
	var rule map[string]any
	if json.Unmarshal(raw, &rule) != nil {
		return
	}
	id, _ := rule["id"].(string)
	if id == "" {
		return
	}
	if _, duplicate := rules[id]; duplicate {
		rules[id] = ""
		return
	}
	delete(rule, "referenced_by_rule_groups")
	rules[id] = ruleDigest(rule)
}

func (r subscriptionRuleRecord) project(rules map[string]string) flow.SubscriptionRuleGroup {
	available := r.Valid && r.Expression != "" && len(r.Rules) > 0 && (r.Type == "" || mediaKinds[r.Type] != 0)
	refs := slices.Clone(r.Rules)
	slices.Sort(refs)
	fingerprints := []string{r.Expression, r.Type, r.Category}
	for _, ref := range refs {
		available = available && rules[ref] != ""
		fingerprints = append(fingerprints, ref, rules[ref])
	}
	return flow.SubscriptionRuleGroup{
		Name: r.Name, Kind: mediaKinds[r.Type], Category: r.Category,
		Available: available, Fingerprint: ruleDigest(fingerprints),
	}
}

func ruleDigest(value any) string {
	raw, _ := json.Marshal(value)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
