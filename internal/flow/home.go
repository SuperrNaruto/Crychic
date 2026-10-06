package flow

import "context"

const (
	actionHome     = "o" // shows the home menu
	actionFeature  = "h" // arg: index into the home features
	actionAskTitle = "q" // typed answer: a title to search for

	msgHomeExpired = "⌛ 这个菜单已失效，请重新 /start。"
	msgAskTitle    = "请直接回复要搜索的片名："
)

// feature is one entry of the home menu.
type feature struct {
	label string
	open  func(ctx context.Context, sess session) Reply
}

// features are the home menu entries, in order.
func (e *Engine) features() []feature {
	return []feature{
		{"搜索订阅", func(_ context.Context, sess session) Reply { return askTitle(sess) }},
		{"发现", func(_ context.Context, sess session) Reply { return e.chartMenu(sess) }},
		{"订阅", e.listSubs},
		{"最新入库", e.latest},
		{"任务进度", e.listTasks},
	}
}

// Home is the menu of everything the bot does.
func (e *Engine) Home(_ context.Context, actor Actor) Reply {
	sess := e.store.create(actor, nil)
	return e.shown(sess.id, e.home(sess))
}

func (e *Engine) home(sess session) Reply {
	e.store.put(sess)
	var buttons []Button
	for i, f := range e.features() {
		buttons = append(buttons, Button{Label: f.label, Data: data(sess.id, actionFeature, i)})
	}
	rows := grid(buttons, menuColumns)
	text := Lines(
		Heading(Plain("🎬 Crychic")),
		Line(Plain("搜索并订阅电影和剧集，入库后通知你。私聊直接发送片名即可搜索。")),
	)
	return Reply{Text: text, Banner: true, Buttons: rows}
}

// chooseHome applies the home menu actions; ok is false for others.
func (e *Engine) chooseHome(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionHome:
		return e.home(sess), true
	case actionFeature:
		features := e.features()
		if p.arg < 0 || p.arg >= len(features) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		sess.menu = true
		return features[p.arg].open(ctx, sess), true
	}
	return Reply{}, false
}

// askTitle prompts for a title to search for.
func askTitle(sess session) Reply {
	return Reply{
		Text:    Sentence(msgAskTitle),
		Buttons: [][]Button{{homeButton(sess.id)}},
		Input:   data(sess.id, actionAskTitle, 0),
	}
}

func homeButton(id uint64) Button {
	return Button{Label: "首页", Data: data(id, actionHome, 0)}
}
