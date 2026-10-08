package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"maps"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type photo struct {
	digest        string
	width, height int
	fileID        string
	reused        bool // sent by file_id instead of uploaded
}

var (
	imageSrc  = regexp.MustCompile(`<img src="([^"]*)"/>`)
	slideshow = regexp.MustCompile(`^<tg-slideshow>(.*?)</tg-slideshow>`)
)

// headerBlocks are the photo or slideshow topping a rich message as
// Telegram returns them, each photo with the file_id it was stored under;
// the fake leaves out the text blocks, which the bot never reads back.
func (m message) headerBlocks() []any {
	var srcs []string
	show := slideshow.FindStringSubmatch(m.text)
	switch {
	case show != nil:
		for _, s := range imageSrc.FindAllStringSubmatch(show[1], -1) {
			srcs = append(srcs, s[1])
		}
	case strings.HasPrefix(m.text, "<img "):
		srcs = []string{imageSrc.FindStringSubmatch(m.text)[1]}
	}
	photos := []any{}
	for _, src := range srcs {
		photos = append(photos, map[string]any{"type": "photo", "photo": []map[string]any{{
			"file_id": m.fileOf(src), "file_unique_id": m.fileOf(src), "width": posterWidth, "height": posterHeight,
		}}})
	}
	if show != nil {
		return []any{map[string]any{"type": "slideshow", "blocks": photos}}
	}
	return photos
}

// fileOf is the file_id Telegram stores an image under: the uploaded or
// reused one for a media reference, one of its own for a fetched URL.
func (m message) fileOf(src string) string {
	if id, ok := strings.CutPrefix(src, "tg://photo?id="); ok {
		return m.files[id]
	}
	return fmt.Sprintf("url-%x", sha256.Sum256([]byte(src)))
}

// seen is the message as its reader sees it: each image as the file
// Telegram shows, whether sent by URL, upload or file_id. Full digests keep
// distinct URLs and uploaded bytes distinct, including image order and count.
func (m message) seen() string {
	return imageSrc.ReplaceAllStringFunc(m.text, func(img string) string {
		return `<img src="` + m.fileOf(imageSrc.FindStringSubmatch(img)[1]) + `"/>`
	})
}

// wire preserves Telegram's distinction between rich messages and photos,
// including the photo metadata attached to callback queries.
func (m message) wire(id int) map[string]any {
	out := map[string]any{
		"message_id": id, "date": messageDate,
		"chat": map[string]any{"id": m.chat, "type": chatType(m.chat)},
	}
	if m.forum {
		out["chat"] = map[string]any{"id": m.chat, "type": "supergroup", "is_forum": true}
	}
	if m.thread != 0 {
		out["message_thread_id"], out["is_topic_message"] = m.thread, true
	}
	if m.photo == nil {
		out["rich_message"] = map[string]any{"blocks": m.headerBlocks()}
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
	f.messages[f.lastMsg] = message{chat: chat, text: caption, rows: rows, photo: &photo{digest: "legacy"}}
	return f.lastMsg
}

func (f *fakeTelegram) readMessage(r *http.Request, method string) (message, error) {
	chat, err := resolveChat(r.FormValue("chat_id"))
	if err != nil {
		return message{}, fmt.Errorf("chat not found")
	}
	m, err := f.addressed(r, method, chat)
	if err != nil {
		return message{}, err
	}
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
		if err := f.checkRichEdit(r, chat); err != nil {
			return m, err
		}
	}
	f.mu.Lock()
	known := maps.Clone(f.fileIDs)
	f.mu.Unlock()
	m, err = readRichMessage(r, m, known)
	f.mu.Lock()
	bad := f.badImage
	f.mu.Unlock()
	if err == nil && bad != "" && strings.Contains(m.text, `<img src="`+bad+`"/>`) {
		err = fmt.Errorf("failed to get HTTP URL content")
	}
	return m, err
}

func (f *fakeTelegram) checkRichEdit(r *http.Request, chat int64) error {
	id, _ := strconv.Atoi(r.FormValue("message_id"))
	old, ok := f.message(id)
	if !ok || old.chat != chat {
		return fmt.Errorf("message to edit not found")
	}
	if old.photo != nil {
		return fmt.Errorf("there is no text in the message to edit")
	}
	if r.FormValue("text") != "" {
		return fmt.Errorf("the bot only edits rich messages")
	}
	return nil
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
// photos it uploads or reuses by the file_ids in known.
func readRichMessage(r *http.Request, m message, known map[string]bool) (message, error) {
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
	m.text, m.files = in.HTML, map[string]string{}
	for _, item := range in.Media {
		if item.Media.Type != "photo" {
			return m, fmt.Errorf("expected a photo")
		}
		if !strings.Contains(in.HTML, `src="tg://photo?id=`+item.ID+`"`) {
			return m, fmt.Errorf("media %s is not used", item.ID)
		}
		p, err := mediaPhoto(r, item.Media.Media, known)
		if err != nil {
			return m, err
		}
		m.media, m.files[item.ID] = append(m.media, p), p.fileID
	}
	return m, nil
}

// mediaPhoto is an uploaded photo (attach://) or one sent by a file_id
// Telegram handed out earlier.
func mediaPhoto(r *http.Request, media string, known map[string]bool) (photo, error) {
	field, upload := strings.CutPrefix(media, "attach://")
	if !upload {
		if !known[media] {
			return photo{}, fmt.Errorf("wrong file identifier/HTTP URL specified")
		}
		return photo{fileID: media, reused: true}, nil
	}
	p, err := readPhoto(r, field)
	if err != nil {
		return p, err
	}
	p.fileID = "file-" + p.digest
	return p, nil
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
	file, _, err := r.FormFile(field)
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
		digest: fmt.Sprintf("%x", sha256.Sum256(data)),
		width:  img.Bounds().Dx(), height: img.Bounds().Dy(),
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
