package flow

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxConditionRunes = 256
	maxConditionTerms = 12
)

func subEditPrompt(sess session, problem string) Reply {
	edit := sess.subEdit
	text := edit.heading(msgSubEditTitle + " · " + editFieldName(edit.field))
	text = append(text, Line(Plain(edit.fieldStatus())))
	if problem != "" {
		text = append(text, Line(Plain(problem)))
	}
	prompt := fmt.Sprintf("回复单行关键词，用 | 分隔多个候选（任一匹配），最多 %d 字、%d 项；按字面匹配，不接收正则表达式。不要输入密钥、链接或服务器路径哦～", maxConditionRunes, maxConditionTerms)
	rows := [][]Button{{{Label: "不限", Data: data(sess.id, actionSubEditClear, edit.options)}}, subEditRestoreRow(sess), {cancelButton(sess.id)}}
	if edit.field == editStart {
		prompt = "直接回复起始集数，最小为 1；不会把略过的集数写成已取得哦～"
		if edit.base.Total > 0 {
			prompt = fmt.Sprintf("直接回复起始集数（1–%d），不会把略过的集数写成已取得哦～", edit.base.Total)
		}
		rows = [][]Button{subEditRestoreRow(sess), {cancelButton(sess.id)}}
	}
	text = append(text, Line(Plain(prompt)))
	return Reply{Text: text, Image: edit.base.Poster, Buttons: rows, Input: data(sess.id, actionSubEditInput, edit.options)}
}

func (e *Engine) answerSubEdit(sess session, typed Typed) Reply {
	if sess.subEdit == nil || sess.screen.reply.Input != typed.Input {
		return Reply{Notice: msgInvalidChoice}
	}
	sess = copySubEdit(sess)
	edit := sess.subEdit
	if edit.field == editStart {
		start, err := strconv.Atoi(episodeNumber(strings.TrimSpace(typed.Text)))
		if err != nil || !validEditStart(start, edit.base.Total) {
			return subEditPrompt(sess, "请输入有效的集数哦～")
		}
		edit.draft.StartEpisode = start
		return e.subEditMenu(sess)
	}
	if edit.field != editInclude && edit.field != editExclude {
		return Reply{Notice: msgInvalidChoice}
	}
	expression, problem := literalCondition(typed.Text)
	if problem != "" {
		return subEditPrompt(sess, problem)
	}
	edit.setExpression(expression)
	if edit.field == editInclude {
		edit.includeWords = strings.TrimSpace(typed.Text)
	} else {
		edit.excludeWords = strings.TrimSpace(typed.Text)
	}
	return e.subEditMenu(sess)
}

func validEditStart(start, total int) bool {
	return start > 0 && (total <= 0 || start <= total)
}

// QuoteMeta produces the portable literal subset shared with Python's re;
// do not reject or reinterpret an existing backend expression using Go's RE2.
func literalCondition(text string) (string, string) {
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxConditionRunes {
		return "", "输入过长或编码无效，缩短后再试哦～"
	}
	if strings.ContainsFunc(text, invalidConditionRune) {
		return "", "请使用单行关键词，不要带换行或控制字符哦～"
	}
	if sensitiveCondition(text) {
		return "", "请勿输入链接、路径或凭据；这次内容没有写入订阅哦～"
	}
	terms := strings.Split(strings.TrimSpace(text), "|")
	if len(terms) > maxConditionTerms {
		return "", "候选关键词太多啦，减少后再试哦～"
	}
	for i, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			return "", "关键词不能为空；想移除限制请点「不限」哦～"
		}
		terms[i] = regexp.QuoteMeta(term)
	}
	return strings.Join(terms, "|"), ""
}

func invalidConditionRune(r rune) bool {
	return unicode.IsControl(r) || r == '\u2028' || r == '\u2029'
}

func sensitiveCondition(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range []string{"/", `\`, "passkey", "cookie", "token", "api_key", "apikey", "secret", "password", "authorization"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
