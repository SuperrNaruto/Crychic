package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (h *harness) nativeNotice(message nativeMessage) {
	h.t.Helper()
	h.tr.add(">> MoviePilot produces a native notification")
	chat, err := resolveChat(message.Recipient.Chat)
	if err != nil {
		h.t.Fatal(err)
	}
	done := h.tg.expect("send:" + fmt.Sprint(chat))
	h.mp.native.publish(message)
	h.wait(done, "native notification delivery")
}

func personalNative(title, text string) nativeMessage {
	return nativeMessage{Title: title, Text: text, Recipient: nativeRecipient{Kind: "personal", Chat: "1001", User: "1001"}}
}

func TestNativeNotificationRichDisplayAndRestart(t *testing.T) {
	h := start(t, scenario{})
	message := personalNative("MP <通知>", "整理完成\n<不可信标签>")
	message.Image, message.Link = dunePoster, "https://example.test/watch/1"
	h.nativeNotice(message)
	h.shows(1, "MP &lt;通知&gt;")
	h.shows(1, "&lt;不可信标签&gt;")
	h.restart()
	h.nativeNotice(personalNative("MP 新通知", "重启后继续接收"))
	if strings.Count(h.tr.String(), "<h3>MP &lt;通知&gt;</h3>") != 1 {
		t.Fatal("native notice replayed")
	}
	raw, err := os.ReadFile(filepath.Join(h.cfg.DataDir, "native-notifications.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "整理完成") {
		t.Fatal("checkpoint contains notification body")
	}
	h.tr.verify(t)
}

func TestNativeChannelKeepsTargetAndTopic(t *testing.T) {
	h := start(t, scenario{notifyChat: "-999"})
	h.sayInTopic(alice, 17, "/start")
	h.nativeNotice(nativeMessage{Title: "MP 频道通知", Text: "使用 MP 指定目标", Recipient: nativeRecipient{Kind: "channel", Chat: "-500", Topic: 17}})
	h.shows(2, "使用 MP 指定目标")
	h.showsInTopic(2, 17)
	h.tr.verify(t)
}

func TestTransfersDoNotGenerateNativeNotifications(t *testing.T) {
	h := start(t, scenario{routes: movieRequestRoutes(), quiet: "0s", stall: "100ms", digest: "mon 12:00"})
	requestMovieDownload(h)
	h.transfers(duneFile())
	h.say(alice, alice, "/requests")
	h.tap(alice, 2, "最近完成")
	h.shows(2, "完成")
	if strings.Contains(h.tr.String(), "<h3>📥 入库啦") {
		t.Fatal("local arrival engine still sends")
	}
	h.tr.verify(t)
}

func TestNativeUnknownSendIsNotReplayed(t *testing.T) {
	h := start(t, scenario{})
	h.tg.loseNextResponse("sendRichMessage")
	h.nativeNotice(personalNative("MP 未知发送", "发送响应丢失"))
	h.restart()
	h.nativeNotice(personalNative("MP 后续通知", "继续处理新消息"))
	if strings.Count(h.tr.String(), "<h3>MP 未知发送</h3>") != 1 {
		t.Fatal("unknown delivery was replayed")
	}
	h.tr.verify(t)
}

func TestNativeWhitelistAndMalformedBatchStayQuiet(t *testing.T) {
	h := start(t, scenario{})
	denied := personalNative("不会显示", "其他用户")
	denied.Recipient = nativeRecipient{Kind: "personal", Chat: "2002", User: "2002"}
	h.mp.native.publish(denied)
	h.nativeNotice(personalNative("MP 允许用户", "仅允许用户收到"))
	h.mp.native.mu.Lock()
	h.mp.native.malformed = true
	h.mp.native.mu.Unlock()
	h.mp.native.publish(personalNative("坏范围", "不能推进"))
	h.say(alice, alice, "/start")
	h.shows(2, "Crychic")
	if strings.Contains(h.tr.String(), "其他用户") {
		t.Fatal("recipient escaped whitelist")
	}
	h.tr.verify(t)
}
