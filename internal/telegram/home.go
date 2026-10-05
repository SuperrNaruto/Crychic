package telegram

import (
	"bytes"
	"context"
	_ "embed"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

const homeFilename = "home.jpg"

//go:embed assets/home.jpg
var homePhoto []byte

// send uploads the bundled home banner, or sends an ordinary text reply.
// A fresh reader per request keeps concurrent chats independent.
func (a *adapter) send(ctx context.Context, chat int64, reply flow.Reply) (*models.Message, error) {
	if reply.Banner {
		return a.api.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:  chat,
			Photo:   &models.InputFileUpload{Filename: homeFilename, Data: bytes.NewReader(homePhoto)},
			Caption: renderHTML(reply.Text), ParseMode: models.ParseModeHTML,
			ReplyMarkup: keyboard(reply.Buttons),
		})
	}
	return a.api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chat, Text: renderHTML(reply.Text), ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: preview(reply.Image), ReplyMarkup: keyboard(reply.Buttons),
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
			data: reply.Follow, shown: renderHTML(reply.Text),
		})
	}
}

func (a *adapter) display(ctx context.Context, t editTarget, reply flow.Reply) (editTarget, error) {
	if reply.Banner {
		_, err := a.api.EditMessageMedia(ctx, &bot.EditMessageMediaParams{
			ChatID: t.chat, MessageID: t.message,
			Media: &models.InputMediaPhoto{
				Media: "attach://" + homeFilename, MediaAttachment: bytes.NewReader(homePhoto),
				Caption: renderHTML(reply.Text), ParseMode: models.ParseModeHTML,
			},
			ReplyMarkup: keyboard(reply.Buttons),
		})
		t.photo = true
		return t, err
	}
	if t.photo {
		return a.replacePhoto(ctx, t, reply)
	}
	_, err := a.api.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: t.chat, MessageID: t.message,
		Text: renderHTML(reply.Text), ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: preview(reply.Image), ReplyMarkup: keyboard(reply.Buttons),
	})
	return t, err
}

// Telegram cannot edit a photo into text. Send the destination first so a
// failed send never removes the menu, then remove the superseded home photo.
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
