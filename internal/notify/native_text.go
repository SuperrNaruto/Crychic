package notify

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const nativeIDBits = 64
const nativeDecimal = 10

var sensitiveNativeText = regexp.MustCompile(`(?i)(api[_-]?key|token|cookie|passkey|password|authorization)\s*[:=]\s*\S+|(?:^|\s)(?:/[\w.-]+){2,}\S*|[A-Za-z]:\\\S+`)
var nativeChannelName = regexp.MustCompile(`^@[A-Za-z0-9_]+$`)

func nativeNotice(message NativeMessage) (flow.Notice, error) {
	if message.Platform != "telegram" {
		return flow.Notice{}, errors.New("native notification platform unsupported")
	}
	actor, err := nativeActor(message.Recipient)
	if err != nil {
		return flow.Notice{}, err
	}
	title := strings.TrimSpace(sensitiveNativeText.ReplaceAllString(message.Title, "[已隐藏]"))
	if title == "" {
		title = "MoviePilot 通知"
	}
	text := flow.Lines(flow.Heading(flow.Plain(title)))
	for _, line := range strings.Split(sensitiveNativeText.ReplaceAllString(message.Text, "[已隐藏]"), "\n") {
		text = append(text, flow.Line(flow.Plain(line)))
	}
	if safeNativeURL(message.Link) {
		text = append(text, flow.Line(flow.Linked(flow.Plain("查看详情"), message.Link)))
	}
	image := ""
	if safeNativeURL(message.Image) {
		image = message.Image
	}
	return flow.Notice{To: actor, Text: text, Image: image}, nil
}

func (n *NativeConsumer) notice(message NativeMessage) (flow.Notice, error) {
	for _, secret := range n.opts.Secrets {
		if secret == "" {
			continue
		}
		message.Title = strings.ReplaceAll(message.Title, secret, "[已隐藏]")
		message.Text = strings.ReplaceAll(message.Text, secret, "[已隐藏]")
		if strings.Contains(message.Image, secret) {
			message.Image = ""
		}
		if strings.Contains(message.Link, secret) {
			message.Link = ""
		}
	}
	return nativeNotice(message)
}

func nativeActor(to NativeRecipient) (flow.Actor, error) {
	if to.Kind == "personal" {
		return nativePersonal(to)
	}
	return nativeChannel(to)
}

func nativePersonal(to NativeRecipient) (flow.Actor, error) {
	id, err := strconv.ParseInt(to.Chat, nativeDecimal, nativeIDBits)
	if err != nil || id <= 0 || to.User != to.Chat || to.Topic != 0 {
		return flow.Actor{}, errors.New("native personal target invalid")
	}
	return flow.Actor{Address: to.Chat, UserID: id}, nil
}

func nativeChannel(to NativeRecipient) (flow.Actor, error) {
	actor := flow.Actor{Address: to.Chat}
	id, err := strconv.ParseInt(to.Chat, nativeDecimal, nativeIDBits)
	if to.Kind != "channel" || to.User != "" || to.Topic < 0 {
		return flow.Actor{}, errors.New("native channel target invalid")
	}
	if (err != nil || id >= 0) && !nativeChannelName.MatchString(to.Chat) {
		return flow.Actor{}, errors.New("native destination invalid")
	}
	if to.Topic > 0 {
		if err != nil {
			return flow.Actor{}, errors.New("native topic requires numeric destination")
		}
		actor.Address += "/" + strconv.Itoa(to.Topic)
	}
	return actor, nil
}

func safeNativeURL(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || !allowedNativeURL(u) {
		return false
	}
	for key := range u.Query() {
		if strings.Contains(strings.ToLower(key), "key") || strings.Contains(strings.ToLower(key), "token") || strings.Contains(strings.ToLower(key), "pass") {
			return false
		}
	}
	return true
}

func allowedNativeURL(u *url.URL) bool {
	return u.Host != "" && u.User == nil && (u.Scheme == "https" || u.Scheme == "http")
}
