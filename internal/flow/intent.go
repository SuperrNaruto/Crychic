package flow

import (
	"strings"
	"unicode"
)

// intentWords open a title search the way MoviePilot's own bot reads them:
// 订阅 and 搜索 search as usual, 下载 searches resources once a target is
// chosen.
var intentWords = map[string]bool{"订阅": false, "搜索": false, "下载": true}

// splitIntent strips a leading 订阅, 搜索 or 下载 (with any colon or space
// after it) from term and tells whether it asked to download; a word with
// nothing after it is the title itself. Without a prefix, keep download.
func splitIntent(term string, download bool) (title string, wantsDownload bool) {
	for word, wantsDownload := range intentWords {
		rest, found := strings.CutPrefix(term, word)
		rest = strings.TrimLeftFunc(rest, func(r rune) bool { return r == ':' || r == '：' || unicode.IsSpace(r) })
		if found && rest != "" {
			return rest, wantsDownload
		}
	}
	return term, download
}

// huntNow searches the resources of the card's target at once, for a title
// searched with 下载. The card becomes the screen 返回 leads back to from
// the releases, as if 搜索资源 had been pressed on it: navigate records it
// (skipped) in place of the search under way.
func (e *Engine) huntNow(sess session, card Reply) Reply {
	sess.skipped = &card
	return e.searching(sess, actionTorrentRun)
}
