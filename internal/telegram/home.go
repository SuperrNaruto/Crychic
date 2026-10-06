package telegram

import (
	"bytes"
	"context"
	_ "embed"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

const (
	homeFilename = "home.jpg"
	homeMediaID  = "home"
	// notModified is Telegram's answer to an edit that changes nothing.
	notModified = "message is not modified"
)

//go:embed assets/home.jpg
var homePhoto []byte

// content is a reply as rich message content: the home menu carries the
// bundled banner as uploaded media, other replies their poster URL. A fresh
// reader per request keeps concurrent chats independent.
func content(reply flow.Reply) *models.InputRichMessage {
	if !reply.Banner {
		return richMessage(reply)
	}
	reply.Image = "tg://photo?id=" + homeMediaID
	rich := richMessage(reply)
	rich.Media = []models.InputRichMessageMedia{{ID: homeMediaID, Media: &models.InputMediaPhoto{
		Media: "attach://" + homeFilename, MediaAttachment: bytes.NewReader(homePhoto),
	}}}
	return rich
}

// withoutPoster is reply minus the posters Telegram failed to fetch, so the
// message still goes out; edits that change nothing are not retried. Telegram
// does not say which gallery poster failed, so the whole gallery goes.
func withoutPoster(reply flow.Reply, err error) (flow.Reply, bool) {
	posters := reply.Image != "" || len(reply.Gallery) > 0
	if err == nil || !posters || strings.Contains(err.Error(), notModified) {
		return reply, false
	}
	reply.Image, reply.Gallery = "", nil
	return reply, true
}

// send sends reply as a new rich message.
func (a *adapter) send(ctx context.Context, chat int64, reply flow.Reply) (*models.Message, error) {
	msg, err := sendRich(ctx, a.api, chat, reply)
	if bare, retry := withoutPoster(reply, err); retry {
		a.log.Warn("poster rejected, sending without it", "image", reply.Image, "gallery", len(reply.Gallery), "err", err)
		msg, err = sendRich(ctx, a.api, chat, bare)
	}
	return msg, err
}

func sendRich(ctx context.Context, api *bot.Bot, chat int64, reply flow.Reply) (*models.Message, error) {
	return api.SendRichMessage(ctx, &bot.SendRichMessageParams{
		ChatID: chat, RichMessage: *content(reply), ReplyMarkup: keyboard(reply.Buttons),
	})
}

// showCallback uses Telegram's message kind, including after a restart,
// and attaches pending input or a follower to the actual rendered message.
func (a *adapter) showCallback(ctx context.Context, cq *models.CallbackQuery, reply flow.Reply) {
	msg := cq.Message.Message
	t := editTarget{chat: msg.Chat.ID, user: cq.From.ID, message: msg.ID, photo: len(msg.Photo) > 0}
	a.unfollow(t.key())
	shown, ok := a.edit(ctx, t, reply)
	if ok && reply.Follow != "" {
		a.follow(follower{
			target: shown, actor: actorOf(cq.From, msg.Chat.ID),
			data: reply.Follow, shown: renderReply(reply),
		})
	}
}

// display edits the conversation message in place.
func (a *adapter) display(ctx context.Context, t editTarget, reply flow.Reply) (editTarget, error) {
	if t.photo {
		return a.replacePhoto(ctx, t, reply)
	}
	err := a.editRich(ctx, t, reply)
	if bare, retry := withoutPoster(reply, err); retry {
		a.log.Warn("poster rejected, editing without it", "image", reply.Image, "gallery", len(reply.Gallery), "err", err)
		err = a.editRich(ctx, t, bare)
	}
	return t, err
}

func (a *adapter) editRich(ctx context.Context, t editTarget, reply flow.Reply) error {
	_, err := a.api.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: t.chat, MessageID: t.message,
		RichMessage: content(reply), ReplyMarkup: keyboard(reply.Buttons),
	})
	return err
}

// A home menu sent as a photo before rich messages cannot become one.
// Send the destination first so a failed send never removes the menu, then
// remove the superseded photo.
func (a *adapter) replacePhoto(ctx context.Context, t editTarget, reply flow.Reply) (editTarget, error) {
	msg, err := a.send(ctx, t.chat, reply)
	if err != nil {
		return t, err
	}
	_, err = a.api.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: t.chat, MessageID: t.message})
	a.logFailure("delete home photo", err)
	t.message, t.photo = msg.ID, false
	return t, nil
}
