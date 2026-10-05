package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type photo struct {
	name, digest  string
	width, height int
	above         bool
}

func (p photo) description() string {
	return fmt.Sprintf(" photo=%s %dx%d sha256=%s caption_above=%t", p.name, p.width, p.height, p.digest, p.above)
}

// wire preserves Telegram's distinction between text and photo captions,
// including the photo metadata attached to callback queries.
func (m message) wire(id int) map[string]any {
	out := map[string]any{
		"message_id": id, "date": messageDate,
		"chat": map[string]any{"id": m.chat, "type": chatType(m.chat)},
	}
	if m.photo == nil {
		out["text"] = m.text
		return out
	}
	out["caption"] = m.text
	out["photo"] = []map[string]any{{
		"file_id": m.photo.digest, "file_unique_id": m.photo.digest,
		"width": m.photo.width, "height": m.photo.height,
	}}
	return out
}

func (f *fakeTelegram) readMessage(r *http.Request, method string) (message, error) {
	chat, _ := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
	m := message{chat: chat, text: r.FormValue("text"), mode: r.FormValue("parse_mode")}
	var markup struct {
		InlineKeyboard [][]button `json:"inline_keyboard"`
	}
	if raw := r.FormValue("reply_markup"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &markup); err != nil {
			return m, err
		}
	}
	m.rows = markup.InlineKeyboard
	if strings.HasPrefix(method, "edit") {
		id, _ := strconv.Atoi(r.FormValue("message_id"))
		old, ok := f.message(id)
		if !ok || old.chat != chat {
			return m, fmt.Errorf("message to edit not found")
		}
		if method == "editMessageText" && old.photo != nil {
			return m, fmt.Errorf("there is no text in the message to edit")
		}
	}
	if method == "sendPhoto" || method == "editMessageMedia" {
		return readPhotoMessage(r, method, m)
	}
	return m, nil
}

func readPhotoMessage(r *http.Request, method string, m message) (message, error) {
	field, above := "photo", r.FormValue("show_caption_above_media") == "true"
	m.text = r.FormValue("caption")
	if method == "editMessageMedia" {
		var media struct {
			Type      string `json:"type"`
			Media     string `json:"media"`
			Caption   string `json:"caption"`
			ParseMode string `json:"parse_mode"`
			Above     bool   `json:"show_caption_above_media"`
		}
		if err := json.Unmarshal([]byte(r.FormValue("media")), &media); err != nil {
			return m, err
		}
		if media.Type != "photo" || !strings.HasPrefix(media.Media, "attach://") {
			return m, fmt.Errorf("expected an uploaded photo")
		}
		field, above = strings.TrimPrefix(media.Media, "attach://"), media.Above
		m.text, m.mode = media.Caption, media.ParseMode
	}
	p, err := readPhoto(r, field)
	p.above = above
	m.photo = &p
	return m, err
}

func readPhoto(r *http.Request, field string) (photo, error) {
	file, header, err := r.FormFile(field)
	if err != nil {
		return photo{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return photo{}, err
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return photo{}, err
	}
	return photo{
		name: header.Filename, digest: fmt.Sprintf("%x", sha256.Sum256(data)),
		width: img.Bounds().Dx(), height: img.Bounds().Dy(),
	}, nil
}

func (f *fakeTelegram) deleteMessage(r *http.Request) error {
	id, _ := strconv.Atoi(r.FormValue("message_id"))
	chat, _ := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
	f.mu.Lock()
	defer f.mu.Unlock()
	old, ok := f.messages[id]
	if !ok || old.chat != chat {
		return fmt.Errorf("message to delete not found")
	}
	delete(f.messages, id)
	f.tr.add(fmt.Sprintf("<< deleteMessage chat=%d message=%d", chat, id))
	return nil
}
