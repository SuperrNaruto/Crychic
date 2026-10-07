package e2e

import (
	"strings"
	"testing"
)

// Selective torrents cover every listed segment without filling the gaps.
func TestSubscriptionDetailSelectiveDownload(t *testing.T) {
	const percent = 37
	tasks := []map[string]any{{
		"hash": "selected-episodes", "progress": percent, "state": "downloading",
		"media": map[string]any{
			"media_source": "themoviedb", "media_id": "1396", "type": "电视剧", "title": "绝命毒师",
			"season": "S02", "episode": "E02、E04",
		},
	}}
	routes := subscriptionDetailRoutes()
	routes[queuePath] = ok("queue_none.json")
	routes[downloadsPath] = ok(writeFixture(t, tasks))
	h := start(t, scenario{routes: routes})
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 1, "1")
	for _, want := range []string{
		"<tr><td>E02</td><td>未入库</td><td>下载中 37%</td></tr>",
		"<tr><td>E03</td><td>已入库</td><td>暂无任务记录</td></tr>",
		"<tr><td>E04</td><td>未入库</td><td>下载中 37%</td></tr>",
	} {
		if !strings.Contains(mustMessage(h, 1).text, want) {
			t.Errorf("selective torrent: expected %s", want)
		}
	}
	tasks[0]["media"].(map[string]any)["episode"] = "E02-E03、E05-E06、E09"
	h.mp.setRoute(downloadsPath, ok(writeFixture(t, tasks)))
	h.tap(alice, 1, "刷新")
	for _, want := range []string{
		"<tr><td>E03</td><td>已入库</td><td>下载中 37%</td></tr>",
		"<tr><td>E04</td><td>未入库</td><td>暂无任务记录</td></tr>",
		"<tr><td>E05–E06</td><td>未入库</td><td>下载中 37%</td></tr>",
		"<tr><td>E07–E08</td><td>未入库</td><td>暂无任务记录</td></tr>",
		"<tr><td>E09</td><td>未入库</td><td>下载中 37%</td></tr>",
	} {
		if !strings.Contains(mustMessage(h, 1).text, want) {
			t.Errorf("mixed episode ranges: expected %s", want)
		}
	}
	h.tr.verify(t)
}

// A combined file supplies its state to every covered episode, while
// library visibility stays independent and /tasks still counts one file.
func TestSubscriptionDetailCombinedTransfer(t *testing.T) {
	const (
		season       = 2
		firstEpisode = 2
		lastEpisode  = 3
	)
	routes := subscriptionDetailRoutes()
	routes[downloadsPath] = ok("downloads_none.json")
	routes[queuePath] = ok(writeFixture(t, []map[string]any{{
		"media": map[string]any{
			"media_source": "themoviedb", "media_id": "1396", "type": "电视剧", "title": "绝命毒师",
		},
		"season": season,
		"tasks": []map[string]any{{
			"state": "running",
			"meta":  map[string]any{"begin_episode": firstEpisode, "end_episode": lastEpisode},
		}},
	}}))
	h := start(t, scenario{routes: routes})
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 1, "1")
	for _, want := range []string{
		"<tr><td>E02</td><td>未入库</td><td>整理中</td></tr>",
		"<tr><td>E03</td><td>已入库</td><td>整理中</td></tr>",
		"<tr><td>E04–E13</td><td>未入库</td><td>暂无任务记录</td></tr>",
	} {
		if !strings.Contains(mustMessage(h, 1).text, want) {
			t.Errorf("combined transfer file: expected %s", want)
		}
	}
	h.say(alice, alice, "/tasks")
	h.tap(alice, 2, "1")
	h.shows(2, "已整理 0/1 个文件")
	want := "<td>E02–E03</td><td>▶️ 整理中</td>"
	if !strings.Contains(mustMessage(h, 2).text, want) {
		t.Errorf("combined file in task view: expected %s", want)
	}
	h.tr.verify(t)
}
