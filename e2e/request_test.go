package e2e

import (
	"net/http"
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

	duneMovie  = "1. 沙丘 (2021)"
	conanShow  = "1. 名侦探柯南 (1996)"
	conanFirst = "第 1 季 · 1216 集"
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
	h.say(alice, alice, "/request 名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	h.tap(alice, 1, "只追新集（第 1216 集起）")
	h.shows(1, "已订阅《名侦探柯南》第 1 季（从第 1216 集开始）")
	h.tr.verify(t)
}

// A typed start episode is range-checked against the season, then confirmed.
func TestStartFromTypedEpisode(t *testing.T) {
	h := start(t, scenario{routes: conanRoutes()})
	h.say(alice, alice, "/request 名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	h.tap(alice, 1, "指定起始集…")
	h.answer(alice, 1, "1300")
	h.shows(1, "「1300」不是有效的集数")
	h.answer(alice, 1, "500")
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "已订阅《名侦探柯南》第 1 季（从第 500 集开始）")
	h.tr.verify(t)
}

// In a group only a reply to the conversation message counts as the answer.
func TestGroupChatterIsNotAnAnswer(t *testing.T) {
	h := start(t, scenario{routes: conanRoutes()})
	h.say(alice, group, "/request 名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	h.tap(alice, 1, "指定起始集…")
	h.chatter(alice, group, "500")
	h.chatter(bob, group, "600")
	h.answerQuoting(alice, 1, "700")
	h.shows(1, "确认订阅《名侦探柯南》第 1 季（从第 700 集开始）")
	h.tr.verify(t)
}

func TestSubscribeMovie(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_dune.json"),
		duneDetails:   ok("detail_dune.json"),
		duneLookup:    ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/request 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "已订阅《沙丘》")
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
	h.say(alice, alice, "/request 绝命毒师")
	h.tap(alice, 1, "1. 绝命毒师 (2008)")
	h.tap(alice, 1, "第 2 季 · 13 集")
	h.tap(alice, 1, "从第 1 集开始")
	h.shows(1, "已订阅《绝命毒师》第 2 季")
	h.tr.verify(t)
}

// Details are cosmetic: without them the card falls back to search metadata.
func TestMissingDetailsDoNotBlockSubscribing(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_dune.json"),
		duneLookup:    ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/request 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "已订阅《沙丘》")
	h.tr.verify(t)
}

func TestAlreadySubscribedEndsEarly(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:  ok("search_dune.json"),
		duneDetails: ok("detail_dune.json"),
		duneLookup:  ok("subscription_existing.json"),
	}})
	h.say(alice, alice, "/request 沙丘")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "已在订阅中")
	h.tr.verify(t)
}

// A double tap on confirm, or a stale button, must not subscribe twice.
func TestConfirmTwiceSubscribesOnce(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_dune.json"),
		duneDetails:   ok("detail_dune.json"),
		duneLookup:    ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/request 沙丘")
	h.tap(alice, 1, duneMovie)
	confirm, _ := findButton(mustMessage(h, 1).rows, "确认订阅")
	h.tap(alice, 1, "确认订阅")
	h.tapData(alice, 1, confirm)
	h.shows(1, "已失效")
	h.tr.verify(t)
}

func TestCancelledRequestCannotResume(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{searchPath: ok("search_dune.json")}})
	h.say(alice, alice, "/request 沙丘")
	pick, _ := findButton(mustMessage(h, 1).rows, duneMovie)
	h.tap(alice, 1, "取消")
	h.shows(1, "已取消")
	h.tapData(alice, 1, pick)
	h.shows(1, "已失效")
	h.tr.verify(t)
}

func TestStrangerIsRefused(t *testing.T) {
	h := start(t, scenario{})
	h.say(stranger, stranger, "/request 沙丘")
	h.shows(1, "你没有使用权限")
	h.tr.verify(t)
}

// In a group, another whitelisted member cannot hijack someone's request.
func TestOnlyRequesterCanChoose(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:  ok("search_dune.json"),
		duneDetails: ok("detail_dune.json"),
		duneLookup:  ok("subscription_none.json"),
	}})
	h.say(alice, group, "/request@crychic_bot 沙丘")
	h.tap(bob, 1, duneMovie)
	h.shows(1, "的搜索结果")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "确认订阅《沙丘》")
	h.tr.verify(t)
}

func TestNothingFound(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{searchPath: ok("empty.json")}})
	h.say(alice, alice, "/request 不存在的电影")
	h.shows(1, "没有找到")
	h.tr.verify(t)
}

func TestWrongAPIKeyIsExplained(t *testing.T) {
	h := start(t, scenario{apiKey: "wrong-key"})
	h.say(alice, alice, "/request 沙丘")
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
	h.say(alice, alice, "/request 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "未识别到媒体信息")
	h.tr.verify(t)
}

func TestMoviePilotOutageIsReported(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: {status: http.StatusInternalServerError, fixture: "server_error.json"},
	}})
	h.say(alice, alice, "/request 沙丘")
	h.shows(1, "MoviePilot 暂时不可用")
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
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_breaking_bad.json"),
		breakDetails:  ok("detail_breaking_bad.json"),
		seasonsPath:   ok("seasons_breaking_bad.json"),
		breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/request 绝命毒师")
	h.tap(alice, 1, "1. 绝命毒师 (2008)")
	h.tap(alice, 1, "多选季…")
	h.tap(alice, 1, "⬜ 第 1 季 · 7 集")
	h.tap(alice, 1, "⬜ 第 2 季 · 13 集")
	h.tap(alice, 1, "☑️ 第 1 季 · 7 集")
	h.tap(alice, 1, "⬜ 第 3 季 · 13 集")
	h.tap(alice, 1, "订阅所选 2 季")
	h.shows(1, "✅ 第 2 季：已订阅")
	h.shows(1, "✅ 第 3 季：已订阅")
	h.arrives(alice, breakingBad.file("S03", "E01"))
	h.shows(2, "第 3 季")
	h.tr.verify(t)
}

// Typing "/" offers the bot's commands; the menu is registered on start.
func TestCommandMenuIsRegistered(t *testing.T) {
	h := start(t, scenario{})
	h.tr.add("<< setMyCommands", h.tg.menu()...)
	h.tr.verify(t)
}

// The home menu leads to every feature; searching from it takes a typed title.
func TestHomeSearchTakesATypedTitle(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{searchPath: ok("search_dune.json")}})
	h.say(alice, alice, "/start")
	h.tap(alice, 1, "🔍 搜索订阅")
	h.answer(alice, 1, "沙丘")
	h.shows(1, "「沙丘」的搜索结果")
	h.tr.verify(t)
}

const (
	subsPath       = "GET /api/v1/subscribe/"
	deleteFirst    = "DELETE /api/v1/subscribe/1"
	cancelBreaking = "取消 1. 绝命毒师 第 2 季"
)

// The subscription list offers to cancel only what the user asked for;
// a cancelled subscription is deleted and its arrivals are no longer
// announced.
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
	h.say(alice, alice, "/request 绝命毒师")
	h.tap(alice, 1, "1. 绝命毒师 (2008)")
	h.tap(alice, 1, "第 2 季 · 13 集")
	h.tap(alice, 1, "从第 1 集开始")
	h.say(alice, alice, "/subs")
	h.tap(alice, 2, cancelBreaking)
	h.tap(alice, 2, "确认取消")
	h.shows(2, "已取消订阅《绝命毒师》第 2 季")
	h.transfers(breakingBad.file("S02", "E01-E13"))
	h.say(bob, bob, "/subs")
	h.tr.verify(t)
}
