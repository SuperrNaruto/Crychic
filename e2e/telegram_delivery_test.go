package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestGroupCommandHonorsBotTarget(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{searchPath: ok("search_dune.json")}})
	h.say(alice, group, "/search@Crychic_Bot 沙丘")
	h.shows(1, "帮你找到这些「沙丘」啦")
	h.chatter(alice, group, "/search@other_bot 沙丘")
	h.transfers()
	const ownTargetOnly = 1
	if got := strings.Count(h.tr.String(), "-> MoviePilot GET /api/v1/media/search?"); got != ownTargetOnly {
		t.Errorf("foreign-target command reached MoviePilot: got %d searches, want %d", got, ownTargetOnly)
	}
	if _, found := h.tg.message(2); found {
		t.Error("Crychic answered a command for another bot")
	}
	h.tap(alice, 1, "取消")
	h.tr.verify(t)
}

// Commands, callbacks, two independent prompts and a persisted arrival all
// stay in the topic that originated them, without changing ordinary chats.
func TestForumConversationKeepsItsTopic(t *testing.T) {
	const firstTopic, secondTopic = 42, 77
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("search_abyss.json"), abyssDetails: ok("detail_abyss.json"),
		seasonsPath: ok("seasons_abyss.json"), abyssLookup: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.sayInTopic(alice, firstTopic, "/start@crychic_bot")
	h.showsInTopic(1, firstTopic)
	h.tap(alice, 1, "搜索")
	h.answerQuoting(alice, 1, "深渊无间")
	h.tap(alice, 1, "指定起始集…")
	h.sayInTopic(alice, secondTopic, "/start@crychic_bot")
	h.showsInTopic(2, secondTopic)
	h.tap(alice, 2, "搜索")
	h.answerQuoting(alice, 1, "5")
	h.shows(1, "要订阅《深渊无间》第 1 季（从第 5 集开始）")
	h.tap(alice, 1, "确认订阅")
	h.answerQuoting(alice, 2, "深渊无间")
	h.tap(alice, 2, "取消")
	h.showsInTopic(1, firstTopic)
	h.showsInTopic(2, secondTopic)
	h.transfers()
	h.restart()
	media := show{title: "深渊无间", year: "2026", id: "301489"}
	h.arrives(group, media.file("S01", "E05"))
	h.shows(3, "入库")
	h.shows(3, "tg://user?id=1001")
	h.showsInTopic(3, firstTopic)
	h.tr.verify(t)
}

// A terminal flow transition is not repeated after Telegram rejects its
// edit: only the unsent reply is retried, until delivered or cancelled.
func TestResourceSearchRecoversItsFinalReply(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"),
		duneLookup: ok("subscription_none.json"), duneTorrents: searchingSites("torrents_dune.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	other, _ := findButton(mustMessage(h, 1).rows, "2")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, searchResources)
	failed := h.tg.failNextEdit(1)
	recovered := h.tg.expect("edit:1")
	h.wait(failed, "Telegram rejecting the completed search")
	h.tapData(alice, 1, other)
	select {
	case <-recovered:
		h.shows(1, "Dune.2021.2160p.UHD.BluRay.REMUX")
	case <-time.After(actionTimeout):
		t.Error("the completed search remains hidden after Telegram recovered")
	}
	h.tap(alice, 1, "取消")
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 2, duneMovie)
	h.tap(alice, 2, searchResources)
	h.wait(h.tg.failNextEdit(2), "Telegram rejecting the next completed search")
	h.tap(alice, 2, "取消")
	h.transfers()
	h.shows(2, "已经取消啦")
	const oneSearchEach = 2
	if got := strings.Count(h.tr.String(), "-> MoviePilot "+duneTorrents); got != oneSearchEach {
		t.Errorf("delivery recovery repeated a resource search: got %d, want %d", got, oneSearchEach)
	}
	h.tr.verify(t)
}

// A duplicate list pick is rejected without abandoning the resource
// search the first pick started, or searching/downloading a second time.
func TestDuplicateDownloadPickKeepsFollowing(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"),
		duneLookup: ok("subscription_none.json"), duneTorrents: searchingSites("torrents_dune.json"),
	}})
	h.say(alice, alice, "下载 沙丘")
	pick, _ := findButton(mustMessage(h, 1).rows, duneMovie)
	other, _ := findButton(mustMessage(h, 1).rows, "2")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "搜索资源")
	h.tapData(alice, 1, pick)
	h.tapData(alice, 1, other)
	waitResourceResult(h)
	h.shows(1, "Dune.2021.2160p.UHD.BluRay.REMUX")
	if strings.Count(h.tr.String(), duneTorrents) != 1 || strings.Contains(h.tr.String(), downloadPath) {
		t.Fatal("a duplicate pick repeated the resource search or started a download")
	}
	h.tap(alice, 1, "取消")
	h.tr.verify(t)
}

func waitResourceResult(h *harness) {
	h.t.Helper()
	const checkEvery = time.Millisecond
	tick := time.NewTicker(checkEvery)
	defer tick.Stop()
	deadline := time.After(actionTimeout)
	for !strings.Contains(h.tr.String(), "<h3>🔍 《沙丘》的资源</h3>") {
		select {
		case <-tick.C:
		case <-deadline:
			h.t.Fatal("the resource search stopped following after a rejected callback")
		}
	}
}
