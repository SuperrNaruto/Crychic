package flow

import "context"

const (
	actionHome     = "o" // shows the home menu
	actionFeature  = "h" // arg: index into the home features
	actionAskTitle = "q" // typed answer: a title to search for
	actionClose    = "c" // closes a browsing screen

	msgHomeExpired = "⌛ 这个菜单睡着啦，发 /start 叫醒我吧～"
	msgAskTitle    = "想找哪部呀？直接回复片名告诉我～"
	msgClosed      = "已经关掉啦～"
)

// feature is one entry of the home menu.
type feature struct {
	label string
	open  func(ctx context.Context, sess session) Reply
}

// features are the home menu entries, in order.
func (e *Engine) features() []feature {
	return []feature{
		{"搜索", func(_ context.Context, sess session) Reply { return askTitle(sess) }},
		{"发现", func(_ context.Context, sess session) Reply { return e.chartMenu(sess) }},
		{"我的订阅", e.listSubs},
		{"最新入库", e.latest},
		{"任务进度", e.listTasks},
		{"追剧日历", e.upcoming},
	}
}

// Home is the menu of everything the bot does.
func (e *Engine) Home(_ context.Context, actor Actor) Reply {
	sess := e.store.create(actor, nil)
	return e.shown(sess.id, e.home(sess))
}

// home leaves the previous title's download intent behind; discovering a
// new title from here asks to subscribe unless resources are requested.
func (e *Engine) home(sess session) Reply {
	sess.download = false
	e.store.put(sess)
	var buttons []Button
	for i, f := range e.features() {
		buttons = append(buttons, Button{Label: f.label, Data: data(sess.id, actionFeature, i)})
	}
	rows := grid(buttons, menuColumns)
	text := Lines(
		Heading(Plain("🎬 Crychic")),
		Line(Plain("想看什么告诉我呀～我帮你找片、订阅，入库了第一时间喊你！私聊直接发片名就能搜哦 (๑•̀ㅂ•́)و✧")),
	)
	return Reply{Text: text, Banner: BannerHome, Buttons: rows}
}

// chooseHome applies the home menu actions; ok is false for others.
func (e *Engine) chooseHome(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionHome:
		return e.home(sess), true
	case actionClose:
		e.store.take(sess.id)
		return Reply{Text: Lines(Heading(Plain("已关闭")), Line(Plain(msgClosed)))}, true
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
		Text:    Lines(Heading(Plain("🔍 搜索")), Line(Plain(msgAskTitle))),
		Buttons: [][]Button{{homeButton(sess.id), cancelButton(sess.id)}},
		Input:   data(sess.id, actionAskTitle, 0),
	}
}

func homeButton(id uint64) Button {
	return Button{Label: "首页", Data: data(id, actionHome, 0)}
}
