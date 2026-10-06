package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type photo struct {
	name, digest  string
	width, height int
}

func (p photo) description() string {
	return fmt.Sprintf(" photo=%s %dx%d sha256=%s", p.name, p.width, p.height, p.digest)
}

// wire preserves Telegram's distinction between rich messages and photos,
// including the photo metadata attached to callback queries.
func (m message) wire(id int) map[string]any {
	out := map[string]any{
		"message_id": id, "date": messageDate,
		"chat": map[string]any{"id": m.chat, "type": chatType(m.chat)},
	}
	if m.photo == nil {
		out["rich_message"] = map[string]any{"blocks": []any{}}
		return out
	}
	out["caption"] = m.text
	out["photo"] = []map[string]any{{
		"file_id": m.photo.digest, "file_unique_id": m.photo.digest,
		"width": m.photo.width, "height": m.photo.height,
	}}
	return out
}

// seedPhoto leaves a photo message with buttons in chat, as the home menu
// was sent before it became a rich message, and returns its id.
func (f *fakeTelegram) seedPhoto(chat int64, caption string, rows [][]button) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastMsg++
	f.messages[f.lastMsg] = message{chat: chat, text: caption, rows: rows, photo: &photo{name: "home.jpg", digest: "legacy"}}
	return f.lastMsg
}

func (f *fakeTelegram) readMessage(r *http.Request, method string) (message, error) {
	chat, _ := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
	m := message{chat: chat}
	var markup struct {
		InlineKeyboard [][]button `json:"inline_keyboard"`
	}
	if raw := r.FormValue("reply_markup"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &markup); err != nil {
			return m, err
		}
	}
	m.rows = markup.InlineKeyboard
	if method == "editMessageText" {
		id, _ := strconv.Atoi(r.FormValue("message_id"))
		old, ok := f.message(id)
		if !ok || old.chat != chat {
			return m, fmt.Errorf("message to edit not found")
		}
		if old.photo != nil {
			return m, fmt.Errorf("there is no text in the message to edit")
		}
		if r.FormValue("text") != "" {
			return m, fmt.Errorf("the bot only edits rich messages")
		}
	}
	m, err := readRichMessage(r, m)
	f.mu.Lock()
	bad := f.badImage
	f.mu.Unlock()
	if err == nil && bad != "" && strings.Contains(m.text, `<img src="`+bad+`"/>`) {
		err = fmt.Errorf("failed to get HTTP URL content")
	}
	return m, err
}

// richInput is the InputRichMessage fields the bot uses.
type richInput struct {
	HTML  string `json:"html"`
	Media []struct {
		ID    string `json:"id"`
		Media struct {
			Type  string `json:"type"`
			Media string `json:"media"`
		} `json:"media"`
	} `json:"media"`
}

// readRichMessage checks the rich HTML as Telegram parses it and reads the
// photos it uploads.
func readRichMessage(r *http.Request, m message) (message, error) {
	var in richInput
	if err := json.Unmarshal([]byte(r.FormValue("rich_message")), &in); err != nil {
		return m, fmt.Errorf("rich_message: %w", err)
	}
	if in.HTML == "" {
		return m, fmt.Errorf("rich message content is empty")
	}
	if err := checkRichHTML(in.HTML); err != nil {
		return m, err
	}
	m.text = in.HTML
	for _, item := range in.Media {
		if item.Media.Type != "photo" || !strings.HasPrefix(item.Media.Media, "attach://") {
			return m, fmt.Errorf("expected an uploaded photo")
		}
		if !strings.Contains(in.HTML, `src="tg://photo?id=`+item.ID+`"`) {
			return m, fmt.Errorf("media %s is not used", item.ID)
		}
		p, err := readPhoto(r, strings.TrimPrefix(item.Media.Media, "attach://"))
		if err != nil {
			return m, err
		}
		m.media = append(m.media, p)
	}
	return m, nil
}

var (
	richTag = regexp.MustCompile(`<(/?)([a-z0-9-]+)[^>]*?(/?)>`)
	// richTags are the documented rich HTML tags the bot may use; void
	// ones never close.
	richTags = map[string]bool{
		"b": false, "i": false, "code": false, "a": false, "h3": false, "h4": false, "p": false,
		"ol": false, "li": false, "blockquote": false, "footer": false,
		"details": false, "summary": false, "ul": false, "mark": false,
		"table": false, "tr": false, "th": false, "td": false, "tg-button-row": false, "tg-button": false,
		"tg-slideshow": false,
		"br":           true, "hr": true, "img": true, "input": true,
	}
)

// checkRichHTML refuses undocumented tags and unbalanced markup, as
// Telegram refuses entities it can't parse.
func checkRichHTML(markup string) error {
	var open []string
	for _, tag := range richTag.FindAllStringSubmatch(markup, -1) {
		closing, name, selfClosed := tag[1] == "/", tag[2], tag[3] == "/"
		void, known := richTags[name]
		switch {
		case !known:
			return fmt.Errorf("can't parse rich message: unsupported tag <%s>", name)
		case void || selfClosed:
		case !closing:
			open = append(open, name)
		case len(open) == 0 || open[len(open)-1] != name:
			return fmt.Errorf("can't parse rich message: unexpected </%s>", name)
		default:
			open = open[:len(open)-1]
		}
	}
	if len(open) > 0 {
		return fmt.Errorf("can't parse rich message: unclosed <%s>", open[len(open)-1])
	}
	return nil
}

// richLines lays rich HTML out a block or line break per transcript line.
var richBreak = regexp.MustCompile(`(</h[1-6]>|</p>|</li>|</ol>|</blockquote>|</summary>|</details>|<ul>|</ul>|</tr>|<table [^>]*>|</table>|<tg-button-row [^>]*>|</tg-button>|</tg-button-row>|</footer>|<hr/>|<img [^>]*/>|<tg-slideshow>|</tg-slideshow>|<ol [^>]*>|<br>)`)

func richLines(markup string) []string {
	return strings.Split(strings.TrimSuffix(richBreak.ReplaceAllString(markup, "$1\n"), "\n"), "\n")
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
