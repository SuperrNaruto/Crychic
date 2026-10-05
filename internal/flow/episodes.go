package flow

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// startChoices offers where a season subscription starts: the beginning,
// the next episode to air (ongoing shows), or an episode the user types.
func startChoices(sess session, season int) Reply {
	rows := [][]Button{{{Label: "从第 1 集开始", Data: data(sess.id, actionConfirm, 0)}}}
	if next := sess.picked.Details.Next; next.Season == season && next.Number > 1 {
		label := fmt.Sprintf("只追新集（第 %d 集起）", next.Number)
		rows[0] = append(rows[0], Button{Label: label, Data: data(sess.id, actionConfirm, next.Number)})
	}
	rows = append(rows,
		[]Button{{Label: "指定起始集…", Data: data(sess.id, actionAskFrom, 0)}},
		[]Button{cancelButton(sess.id)},
	)
	var facts []string
	if count := sess.episodeCount(season); count > 0 {
		facts = append(facts, fmt.Sprintf("共 %d 集", count))
	}
	if held := sess.library.Episodes[season]; len(held) > 0 {
		facts = append(facts, "媒体库已有 "+EpisodeRanges(held))
	}
	total := ""
	if len(facts) > 0 {
		total = "（" + strings.Join(facts, "，") + "）"
	}
	question := fmt.Sprintf("确认订阅%s%s？", targetName(*sess.target), total)
	return sess.picked.reply(Line(Strong(question)), rows)
}

// askStart prompts for a typed start episode; problem explains why the
// previous answer was rejected.
func askStart(sess session, problem string) Reply {
	if sess.target == nil || sess.target.Season == nil {
		return Reply{Notice: msgInvalidChoice}
	}
	prompt := "请直接回复起始集数"
	if count := sess.episodeCount(*sess.target.Season); count > 0 {
		prompt += fmt.Sprintf("（1–%d）", count)
	}
	reply := sess.picked.reply(Line(Plain(problem+prompt+"：")), [][]Button{{cancelButton(sess.id)}})
	reply.Input = data(sess.id, actionAskFrom, 0)
	return reply
}

// Typed is text a user typed in answer to a Reply with Input.
type Typed struct {
	Input string // the Reply's Input
	Text  string
}

// Answer takes the text a user typed in response to a Reply with Input.
func (e *Engine) Answer(ctx context.Context, actor Actor, typed Typed) Reply {
	id, p, ok := parseData(typed.Input)
	if !ok || (p.action != actionAskFrom && p.action != actionAskTitle) {
		return Reply{Notice: msgInvalidChoice}
	}
	sess, unlock, ok := e.lockSession(ctx, actor, id)
	defer unlock()
	if !ok {
		return expired(p.action)
	}
	if sess.owner.UserID != actor.UserID || sess.owner.Address != actor.Address {
		return Reply{Notice: msgNotYours}
	}
	text := strings.TrimSpace(typed.Text)
	if p.action == actionAskTitle {
		return e.search(ctx, sess, text)
	}
	from, err := strconv.Atoi(text)
	if err != nil || from < 1 || !sess.validStart(from) {
		return askStart(sess, fmt.Sprintf("⚠️「%s」不是有效的集数。", text))
	}
	target := *sess.target
	target.StartEpisode = from
	return confirmCard(sess, target)
}

// validStart reports whether from is an acceptable start episode for the
// session's target: 0 always; for a season, 1..its episode count when known.
func (sess session) validStart(from int) bool {
	if from == 0 {
		return true
	}
	if from < 0 || sess.target == nil || sess.target.Season == nil {
		return false
	}
	count := sess.episodeCount(*sess.target.Season)
	return count == 0 || from <= count
}

// wholeSeasonHeld reports whether every episode of s is in the library.
func (sess session) wholeSeasonHeld(s Season) bool {
	return s.EpisodeCount > 0 && len(sess.library.Episodes[s.Number]) >= s.EpisodeCount
}

// episodeCount is the number of episodes in season, 0 when unknown.
func (sess session) episodeCount(season int) int {
	for _, s := range sess.seasons {
		if s.Number == season {
			return s.EpisodeCount
		}
	}
	return 0
}
