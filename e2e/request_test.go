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

	duneMovie = "电影 · 沙丘 (2021)"
)

func TestSubscribeMovie(t *testing.T) {
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

func TestSubscribeOneSeasonOfShow(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_breaking_bad.json"),
		seasonsPath:   ok("seasons_breaking_bad.json"),
		breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/request 绝命毒师")
	h.tap(alice, 1, "电视剧 · 绝命毒师 (2008)")
	h.tap(alice, 1, "第 2 季 · 13 集")
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "已订阅《绝命毒师》第 2 季")
	h.tr.verify(t)
}

func TestAlreadySubscribedEndsEarly(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("search_dune.json"),
		duneLookup: ok("subscription_existing.json"),
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
		searchPath: ok("search_dune.json"),
		duneLookup: ok("subscription_none.json"),
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
