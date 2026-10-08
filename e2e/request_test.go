package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const (
	searchPath    = "GET /api/v1/media/search"
	seasonsPath   = "GET /api/v1/media/seasons"
	subscribePath = "POST /api/v1/subscribe/"
	duneLookup    = "GET /api/v1/subscribe/media/438631"
	breakingQuery = "GET /api/v1/subscribe/media/1396"
	duneDetails   = "GET /api/v1/media/438631"
	breakDetails  = "GET /api/v1/media/1396"

	conanLookup  = "GET /api/v1/subscribe/media/30983"
	conanDetails = "GET /api/v1/media/30983"

	duneMovie  = "1"
	dune2Movie = "4"
	conanShow  = "1"
	conanFirst = "第 1 季"
)

// conanRoutes serve an ongoing show with one 1216-episode season whose next
// episode (1216) has not aired yet.
func conanRoutes() map[string]route {
	return map[string]route{
		searchPath:    ok("search_conan.json"),
		conanDetails:  ok("detail_conan.json"),
		seasonsPath:   ok("seasons_conan.json"),
		conanLookup:   ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}
}

// Someone who has watched an ongoing show only wants new episodes.
func TestFollowOnlyNewEpisodes(t *testing.T) {
	h := start(t, scenario{routes: conanRoutes()})
	h.say(alice, alice, "/search 名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	h.tap(alice, 1, "只追新集（第 1216 集起）")
	h.shows(1, "帮你订好《名侦探柯南》第 1 季（从第 1216 集开始）")
	h.tr.verify(t)
}

// A typed start episode is range-checked against the season, then confirmed.
func TestStartFromTypedEpisode(t *testing.T) {
	h := start(t, scenario{routes: conanRoutes()})
	h.say(alice, alice, "/search 名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	h.tap(alice, 1, "指定起始集…")
	h.answer(alice, 1, "沙丘")
	h.shows(1, "「沙丘」好像不是有效的集数")
	h.answer(alice, 1, "1300")
	h.shows(1, "「1300」好像不是有效的集数")
	h.answer(alice, 1, "第５集")
	h.shows(1, "从第 5 集开始")
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "指定起始集…")
	h.answer(alice, 1, "５００")
	h.shows(1, "从第 500 集开始")
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "帮你订好《名侦探柯南》第 1 季（从第 500 集开始）")
	h.tr.verify(t)
}

// In a group only a reply to the conversation message counts as the answer.
func TestGroupChatterIsNotAnAnswer(t *testing.T) {
	h := start(t, scenario{routes: conanRoutes()})
	h.say(alice, group, "/search 名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	h.tap(alice, 1, "指定起始集…")
	h.chatter(alice, group, "500")
	h.chatter(bob, group, "600")
	h.answerQuoting(alice, 1, "700")
	h.shows(1, "要订阅《名侦探柯南》第 1 季（从第 700 集开始）")
	h.tr.verify(t)
}

func TestSubscribeMovie(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_dune.json"),
		duneDetails:   ok("detail_dune.json"),
		duneLookup:    ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "帮你订好《沙丘》")
	h.tr.verify(t)
}

// A poster Telegram cannot fetch costs only the poster, never the card.
func TestUnreachablePosterKeepsTheCard(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:  ok("search_dune.json"),
		duneDetails: ok("detail_dune.json"),
		duneLookup:  ok("subscription_none.json"),
	}})
	h.tg.refuseImage(dunePoster)
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "要订阅《沙丘》")
	h.tr.verify(t)
}

// After a callback completes, Telegram can refuse its cached poster. The
// gallery then retries by URL, and the next card reuses the recovered file_id.
func TestRefusedPosterFileIsSentAfresh(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:  ok("search_dune.json"),
		duneDetails: ok("detail_dune.json"),
		duneLookup:  ok("subscription_none.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tg.forgetFiles()
	h.tap(alice, 1, "返回")
	h.shows(1, `<img src="`+dunePoster+`"/>`)
	h.tap(alice, 1, duneMovie)
	h.shows(1, "要订阅《沙丘》")
	media := mustMessage(h, 1).media
	if len(media) != 1 || !media[0].reused {
		t.Fatal("the recovered poster must be reused by file_id")
	}
	h.tr.verify(t)
}

func TestSubscribeOneSeasonOfShow(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_breaking_bad.json"),
		breakDetails:  ok("detail_breaking_bad.json"),
		seasonsPath:   ok("seasons_breaking_bad.json"),
		breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 绝命毒师")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 2 季")
	h.tap(alice, 1, "从第 1 集开始")
	h.shows(1, "帮你订好《绝命毒师》第 2 季")
	h.tr.verify(t)
}

// Details are cosmetic: without them the card falls back to search metadata.
func TestMissingDetailsDoNotBlockSubscribing(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_dune.json"),
		duneLookup:    ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "帮你订好《沙丘》")
	h.tr.verify(t)
}

func TestAlreadySubscribedEndsEarly(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:  ok("search_dune.json"),
		duneDetails: ok("detail_dune.json"),
		duneLookup:  ok("subscription_existing.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "早就订阅上啦")
	h.mp.setRoute(duneLookup, ok(writeFixture(t, map[string]any{
		"id": 1, "state": "S",
	})))
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 2, duneMovie)
	h.shows(2, "目前已暂停")
	if _, offered := findButton(mustMessage(h, 2).rows, "确认订阅"); offered {
		t.Fatal("a paused existing subscription must not be recreated")
	}
	h.tr.verify(t)
}

// A double tap on confirm, or a stale button, must not subscribe twice; a
// confirmation from a pick before cannot subscribe the next pick from the
// same list, whose own 确认订阅 looks the same but was never pressed.
func TestConfirmTwiceSubscribesOnce(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:         ok("search_dune.json"),
		duneDetails:        ok("detail_dune.json"),
		duneLookup:         ok("subscription_none.json"),
		dunePartTwoDetails: ok("detail_dune.json"),
		dunePartTwoLookup:  ok("subscription_none.json"),
		subscribePath:      ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	confirm, _ := findButton(mustMessage(h, 1).rows, "确认订阅")
	h.tap(alice, 1, "确认订阅")
	h.tapData(alice, 1, confirm)
	h.shows(1, "帮你订好《沙丘》")
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, dune2Movie)
	h.shows(1, "要订阅《沙丘2》")
	h.tapData(alice, 1, confirm)
	if writes := strings.Count(h.tr.String(), subscribePath); writes != 1 {
		t.Errorf("subscription writes = %d, want 1: a stale confirmation subscribed the next pick", writes)
	}
	h.tr.verify(t)
}

func TestCancelledRequestCannotResume(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{searchPath: ok("search_dune.json")}})
	h.say(alice, alice, "/search 沙丘")
	pick, _ := findButton(mustMessage(h, 1).rows, duneMovie)
	h.tap(alice, 1, "取消")
	h.shows(1, "已经取消啦")
	h.tapData(alice, 1, pick)
	h.shows(1, "过期啦")
	h.tr.verify(t)
}

func TestStrangerIsRefused(t *testing.T) {
	h := start(t, scenario{})
	h.say(stranger, stranger, "沙丘")
	h.shows(1, "你还没有使用权限")
	h.say(stranger, stranger, "/start")
	h.shows(2, "你还没有使用权限")
	h.tr.verify(t)
}

// Every recognized private command releases an older episode prompt, so
// later titles are not mistaken for answers to the abandoned conversation.
func TestPrivateCommandsReplacePendingAnswer(t *testing.T) {
	h := start(t, scenario{
		routes: conanRoutes(),
		searches: map[string]string{
			"名侦探柯南": "search_conan.json",
			"沙丘":    "search_dune.json",
		},
	})
	h.say(alice, alice, "/search 名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	h.tap(alice, 1, "指定起始集…")
	h.say(alice, alice, "/start")
	h.say(alice, alice, "沙丘")
	h.shows(3, "这些「沙丘」啦")
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "指定起始集…")
	h.say(alice, alice, "/search 沙丘")
	h.say(alice, alice, "沙丘")
	h.shows(5, "这些「沙丘」啦")
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "指定起始集…")
	h.say(alice, alice, "/trending")
	h.say(alice, alice, "沙丘")
	h.shows(7, "这些「沙丘」啦")
	h.tr.verify(t)
}

// An explicit reply to an older prompt cannot answer the newest private
// prompt. The warning preserves both, and replying to the current one works.
func TestPrivateQuotedAnswerKeepsCurrentPrompt(t *testing.T) {
	routes := conanRoutes()
	routes[abyssDetails] = ok("detail_abyss.json")
	routes[abyssLookup] = ok("subscription_none.json")
	h := start(t, scenario{routes: routes})
	h.say(alice, alice, "/search 名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	h.tap(alice, 1, "指定起始集…")
	h.mp.setRoute(searchPath, ok("search_abyss.json"))
	h.mp.setRoute(seasonsPath, ok("seasons_abyss.json"))
	h.say(alice, alice, "/search 深渊无间")
	h.tap(alice, 2, "指定起始集…")
	older, current := mustMessage(h, 1).seen(), mustMessage(h, 2).seen()
	h.tr.add(fmt.Sprintf(">> user %d in chat %d quoting message 1: 5", alice, alice))
	wrong := chatMessage{user: alice, chat: alice, text: "5", replyTo: 1}
	h.wait(h.tg.push(wrong.update(), fmt.Sprintf("send:%d", alice)), "a mismatched reply warning")
	h.shows(3, "你回复的不是我正在等的那条提问")
	if mustMessage(h, 1).seen() != older || mustMessage(h, 2).seen() != current {
		t.Error("a mismatched reply changed a conversation")
	}
	h.answerQuoting(alice, 2, "7")
	h.shows(2, "要订阅《深渊无间》第 1 季（从第 7 集开始）")
	h.tap(alice, 2, "确认订阅")
	h.shows(2, "帮你订好《深渊无间》第 1 季（从第 7 集开始）")
	h.tr.verify(t)
}

// In a group, another whitelisted member cannot hijack someone's request.
func TestOnlyRequesterCanChoose(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:  ok("search_dune.json"),
		duneDetails: ok("detail_dune.json"),
		duneLookup:  ok("subscription_none.json"),
	}})
	h.say(alice, group, "/search@crychic_bot 沙丘")
	h.tap(bob, 1, duneMovie)
	h.shows(1, "帮你找到这些")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "要订阅《沙丘》")
	h.tr.verify(t)
}

func TestNothingFound(t *testing.T) {
	h := start(t, scenario{searches: map[string]string{
		"不存在的电影": "empty.json",
		"沙丘":     "search_dune.json",
	}})
	h.say(alice, alice, "/search 不存在的电影")
	h.shows(1, "什么都没找到")
	h.answer(alice, 1, "沙丘")
	h.shows(1, "这些「沙丘」啦")
	h.tr.verify(t)
}

func TestWrongAPIKeyIsExplained(t *testing.T) {
	h := start(t, scenario{apiKey: "wrong-key"})
	h.say(alice, alice, "/search 沙丘")
	h.shows(1, "API Key")
	h.tr.verify(t)
}

func TestMoviePilotRefusalIsRelayed(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_dune.json"),
		duneDetails:   ok("detail_dune.json"),
		duneLookup:    ok("subscription_none.json"),
		subscribePath: ok("subscribe_rejected.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "未识别到媒体信息")
	h.tr.verify(t)
}

func TestMoviePilotOutageIsReported(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: {status: http.StatusInternalServerError, fixture: "server_error.json"},
	}})
	h.say(alice, alice, "/search 沙丘")
	h.shows(1, "MoviePilot 好像打了个盹")
	retry, _ := findButton(mustMessage(h, 1).rows, "重试")
	h.tap(alice, 1, "重试")
	h.mp.setRoute(searchPath, ok("search_dune.json"))
	h.tap(alice, 1, "重试")
	h.shows(1, "这些「沙丘」啦")
	h.tapData(alice, 1, retry)
	h.shows(1, "这些「沙丘」啦")
	h.tr.verify(t)
}

func mustMessage(h *harness, id int) message {
	h.t.Helper()
	msg, ok := h.tg.message(id)
	if !ok {
		h.t.Fatalf("message %d was never sent", id)
	}
	return msg
}

// Several seasons are subscribed in one go, each from its first episode,
// and each is watched for arrivals.
func TestSubscribeSeveralSeasons(t *testing.T) {
	h := start(t, scenario{notifyChat: channelName, routes: map[string]route{
		searchPath:    ok("search_breaking_bad.json"),
		breakDetails:  ok("detail_breaking_bad.json"),
		seasonsPath:   ok("seasons_breaking_bad.json"),
		breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 绝命毒师")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "多选季…")
	h.tap(alice, 1, "第 1 季")
	h.tap(alice, 1, "第 2 季")
	h.tap(alice, 1, "✓ 第 1 季")
	h.tap(alice, 1, "第 3 季")
	h.tap(alice, 1, "订阅所选 2 季")
	h.shows(1, "<td><b>第 2 季</b></td><td>✅ 已订阅</td>")
	h.shows(1, "<td><b>第 3 季</b></td><td>✅ 已订阅</td>")
	h.shows(1, "通知频道")
	h.arrives(noticeChannel, breakingBad.file("S03", "E01"))
	h.shows(2, "第 3 季")
	h.tr.verify(t)
}

// Typing "/" offers the bot's commands; the menu is registered on start,
// also for each whitelisted user's chat, where a menu an earlier program
// left would otherwise outrank it.
func TestCommandMenuIsRegistered(t *testing.T) {
	h := start(t, scenario{})
	h.tr.add("<< setMyCommands", h.tg.menu(defaultScope)...)
	for _, user := range []int64{alice, bob} {
		scope := fmt.Sprintf(`{"type":"chat","chat_id":%d}`, user)
		h.tr.add("<< setMyCommands scope="+scope, h.tg.menu(scope)...)
	}
	h.tr.verify(t)
}

// The home menu leads to every feature; searching from it takes a typed title.
func TestHomeSearchTakesATypedTitle(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{searchPath: ok("search_dune.json")}})
	h.say(alice, alice, "/start")
	if len(mustMessage(h, 1).media) == 0 {
		t.Fatal("the home menu must include the banner photo")
	}
	h.tap(bob, 1, "搜索")
	h.tap(alice, 1, "搜索")
	h.tap(alice, 1, "首页")
	h.tap(alice, 1, "搜索")
	h.answer(alice, 1, "沙丘")
	h.shows(1, "这些「沙丘」啦")
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "首页")
	h.restart()
	h.tap(alice, 1, "搜索")
	h.shows(1, "这个菜单睡着啦")
	h.tr.verify(t)
}

// A home menu sent as a photo by an earlier version cannot be edited into
// a rich message: its buttons answer with a new message and the photo goes.
func TestOldHomePhotoIsReplaced(t *testing.T) {
	h := start(t, scenario{})
	old := h.tg.seedPhoto(alice, "🎬 Crychic", [][]button{{{Text: "搜索订阅", CallbackData: "424242:h:0"}}})
	h.tap(alice, old, "搜索订阅")
	if _, ok := h.tg.message(old); ok {
		t.Fatal("the old home photo must be removed")
	}
	h.shows(old+1, "这个菜单睡着啦")
	h.tr.verify(t)
}

const (
	subsPath    = "GET /api/v1/subscribe/"
	deleteFirst = "DELETE /api/v1/subscribe/1"
)

// The subscription list offers one cancel button, which lists only what
// the user asked for; a cancelled subscription is deleted and its arrivals
// are no longer announced. Someone who asked for nothing gets no button.
func TestCancelOwnSubscription(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_breaking_bad.json"),
		breakDetails:  ok("detail_breaking_bad.json"),
		seasonsPath:   ok("seasons_breaking_bad.json"),
		breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
		subsPath:      ok("subscriptions.json"),
		deleteFirst:   ok("subscribe_deleted.json"),
	}})
	h.say(alice, alice, "/search 绝命毒师")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 2 季")
	h.tap(alice, 1, "从第 1 集开始")
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 2, "取消订阅")
	h.tap(alice, 2, "1")
	h.tap(alice, 2, "返回")
	h.shows(2, "只能取消你请求的订阅")
	h.tap(alice, 2, "返回")
	h.shows(2, "订阅清单")
	h.tap(alice, 2, "取消订阅")
	h.tap(alice, 2, "1")
	h.tap(alice, 2, "确认取消")
	h.shows(2, "《绝命毒师》第 2 季的订阅已经取消啦")
	h.tap(alice, 2, "返回")
	h.shows(2, "订阅清单")
	h.transfers(breakingBad.file("S02", "E01-E13"))
	h.say(bob, bob, "/subscribe")
	if _, found := findButton(mustMessage(h, 3).rows, "取消订阅"); found {
		t.Fatal("bob asked for nothing, so there must be nothing for him to cancel")
	}
	h.tr.verify(t)
}

// A subscription the user asked for can be paused and resumed from its
// details, which then show its state read afresh; MoviePilot's refusal is
// relayed and keeps the details. Nobody else gets the button.
func TestPauseOwnSubscription(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_breaking_bad.json"),
		breakDetails:  ok("detail_breaking_bad.json"),
		seasonsPath:   ok("seasons_breaking_bad.json"),
		breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
		subsPath:      ok("subscriptions.json"),
	}})
	h.say(alice, alice, "/search 绝命毒师")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 2 季")
	h.tap(alice, 1, "从第 1 集开始")
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 2, "1")
	h.tap(alice, 2, "暂停订阅")
	h.shows(2, "已暂停")
	h.tap(alice, 2, "恢复订阅")
	h.shows(2, "订阅中")
	h.mp.setRoute("PUT /api/v1/subscribe/status/1", ok("subscribe_gone.json"))
	h.tap(alice, 2, "暂停订阅")
	h.shows(2, "订阅中")
	h.say(bob, bob, "/subscribe")
	h.tap(bob, 3, "1")
	if _, found := findButton(mustMessage(h, 3).rows, "暂停订阅"); found {
		t.Fatal("bob asked for nothing, so there must be nothing for him to pause")
	}
	h.tr.verify(t)
}

// Subscription buttons name their subscription, not its place in the list:
// once a later read dropped season 2, or reused season 3's id for season 4,
// its stale 暂停订阅 and 确认取消 never reach the replacement.
func TestStaleButtonsNeverReachAnotherSubscription(t *testing.T) {
	h := requestedBreakingSeasons(t)
	h.say(alice, alice, "/subscribe")
	pause, cancel := subscriptionButtons(h, 3, "1")
	h.tap(alice, 3, "返回")
	h.tap(alice, 3, "首页")
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{breakingSeason(2, 3)})))
	h.tap(alice, 3, "我的订阅")
	h.shows(3, "第 3 季")
	h.tapData(alice, 3, pause)
	h.tapData(alice, 3, cancel)
	pause, cancel = subscriptionButtons(h, 3, "1")
	h.tap(alice, 3, "返回")
	h.tap(alice, 3, "首页")
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{breakingSeason(2, 4)})))
	h.tap(alice, 3, "我的订阅")
	h.shows(3, "第 4 季")
	if strings.Contains(mustMessage(h, 3).text, "你请求的") {
		t.Error("a reused id grants ownership of another subscription")
	}
	h.tapData(alice, 3, pause)
	h.tapData(alice, 3, cancel)
	if subscriptionWrites(h) != 0 {
		t.Error("a stale button reached a replacement subscription")
	}
	h.tr.verify(t)
}

// A confirmation on a pre-restart card cannot consume a new conversation,
// even when its owner is also the new conversation's owner.
func TestOldConfirmationCannotSubscribeAfterRestart(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"),
		duneLookup: ok("subscription_none.json"), subscribePath: ok("subscribe_created.json"),
		diggerDetails: ok("detail_digger.json"), diggerLookup: ok("subscription_none.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.restart()
	h.mp.setRoute(searchPath, ok("search_digger.json"))
	h.say(alice, alice, "/search 挖掘者")
	h.tap(alice, 2, "1")
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "过期啦")
	h.tap(alice, 2, "确认订阅")
	h.shows(2, "帮你订好《挖掘者》")
	h.tr.verify(t)
}

// The newest library items link to where they can be watched.
func TestLatestLinksToTheMediaServer(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{latestPath: ok("latest.json")}})
	h.say(alice, alice, "/newly_added")
	h.shows(1, `<a href="https://emby.example.com/web/index.html#!/item?id=118&amp;context=home"><b>《颂乐人偶》</b></a>`)
	h.tr.verify(t)
}

// 返回 retraces a request screen by screen, as each was, without asking
// MoviePilot again; the first screen has nowhere to go back to, and a
// request walked back into still subscribes.
func TestBackRetracesTheRequest(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_breaking_bad.json"),
		breakDetails:  ok("detail_breaking_bad.json"),
		seasonsPath:   ok("seasons_breaking_bad.json"),
		breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 绝命毒师")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 2 季")
	h.tap(alice, 1, "指定起始集…")
	h.tap(alice, 1, "返回")
	h.shows(1, "要订阅《绝命毒师》第 2 季")
	h.tap(alice, 1, "返回")
	h.shows(1, "想订哪一季呀")
	h.tap(alice, 1, "返回")
	h.shows(1, "这些「绝命毒师」啦")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 2 季")
	h.tap(alice, 1, "从第 1 集开始")
	h.shows(1, "帮你订好《绝命毒师》第 2 季")
	h.tr.verify(t)
}
