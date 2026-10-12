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
	// MoviePilot's 入库 template joins facts with 「，」; 简介 keeps its own commas.
	message := personalNative("沙丘 (2021) <已入库>",
		"类型：电影，类别：<外语电影>，质量：WEB-DL 1080p，共1个文件，大小：12.3G\n整理完成\n简介：沙漠，香料，命运")
	message.Image, message.Link = dunePoster, "https://example.test/watch/1"
	h.nativeNotice(message)
	h.shows(1, "📥 沙丘 (2021) &lt;已入库&gt;")
	h.shows(1, "<td>&lt;外语电影&gt;</td>")
	h.shows(1, "<td></td><td>共1个文件</td>")
	h.shows(1, "<td>沙漠，香料，命运</td>")
	h.restart()
	h.nativeNotice(personalNative("MP 新通知", "重启后继续接收"))
	if strings.Count(h.tr.String(), "<h3>📥 沙丘 (2021) &lt;已入库&gt;</h3>") != 1 {
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

func TestNativeUnknownSendIsNotReplayed(t *testing.T) {
	h := start(t, scenario{})
	lost := h.tg.loseNextResponse("sendRichMessage")
	h.nativeNotice(personalNative("MP 未知发送", "发送响应丢失"))
	h.wait(lost, "the lost response after displaying the native notice")
	h.restart()
	h.nativeNotice(personalNative("MP 后续通知", "继续处理新消息"))
	if strings.Count(h.tr.String(), "<h3>🔔 MP 未知发送</h3>") != 1 {
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

// MoviePilot's transfer failure notice puts paths right after full-width
// colons and joins several with 、; an older plugin stored them unhidden.
func TestNativeTransferFailureHidesServerPaths(t *testing.T) {
	h := start(t, scenario{})
	text := "失败阶段：整理\n源文件：/downloads/My Show (2024)/S01E02.mkv\n目标路径：/媒体/剧集/My Show\n" +
		"原因：目标已存在\n如果按钮不可用，可回复：\n/redo 12"
	h.nativeNotice(personalNative("My Show S01E02 入库失败！", text))
	h.nativeNotice(personalNative("My Show 入库失败（2 个文件）", "失败文件：2 个\n源文件：/downloads/a.mkv、/downloads/b.mkv\n"+
		"原因：媒体库根目录：/srv/My Show (2024)/c.mkv 不可写\n音轨 国语/粤语/英语，无法写入 (/srv/media/d.mkv)"))
	for _, path := range []string{"/downloads", "/媒体", "/srv", "My Show (2024)"} {
		if strings.Contains(h.tr.String(), path) {
			t.Fatalf("server path %s reached the chat", path)
		}
	}
	h.shows(1, "/redo 12")
	h.shows(2, "国语/粤语/英语")
	h.tr.verify(t)
}
