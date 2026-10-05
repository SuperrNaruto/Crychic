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
		{"🔍 搜索订阅", func(_ context.Context, sess session) Reply { return askTitle(sess) }},
		{"🔥 发现", func(_ context.Context, sess session) Reply { return e.chartMenu(sess) }},
		{"📚 订阅", e.listSubs},
		{"📋 任务进度", e.listTasks},
	}
}

// Home is the menu of everything the bot does.
func (e *Engine) Home(_ context.Context, actor Actor) Reply {
	return e.home(e.store.create(actor, nil))
}

func (e *Engine) home(sess session) Reply {
	e.store.put(sess)
	var rows [][]Button
	for i, f := range e.features() {
		b := Button{Label: f.label, Data: data(sess.id, actionFeature, i)}
		if i%homeColumns == 0 {
			rows = append(rows, []Button{b})
			continue
		}
		rows[len(rows)-1] = append(rows[len(rows)-1], b)
	}
	text := Lines(
		Line(Strong("🎬 Crychic")),
		Line(Plain("搜索并订阅电影和剧集，入库后通知你。")),
	)
	return Reply{Text: text, Buttons: rows}
}

// homeColumns is how many home buttons share a row.
const homeColumns = 2

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
	return Button{Label: "🏠 首页", Data: data(id, actionHome, 0)}
}
