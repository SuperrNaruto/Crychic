package flow

import "strconv"

const (
	decimalBase = 10
	// maxNamedSeason is the largest season written in Chinese numerals here.
	maxNamedSeason = 99
)

var chineseDigits = [decimalBase]string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"}

// SeasonSuffix is what MoviePilot's search appends to a TMDB show's title
// when the search term names season n: 青之箱 becomes 青之箱 第二季
// (cn2an's lower-case numerals), though the show itself is still 青之箱.
func SeasonSuffix(n int) string {
	return " 第" + chineseNumber(n) + "季"
}

// chineseNumber writes 1…99 the way cn2an.an2cn(n, "low") does: 二, 十,
// 十一, 二十一; other numbers stay digits.
func chineseNumber(n int) string {
	if n < 1 || n > maxNamedSeason {
		return strconv.Itoa(n)
	}
	tens, ones := n/decimalBase, n%decimalBase
	s := ""
	switch {
	case tens == 1:
		s = "十"
	case tens > 1:
		s = chineseDigits[tens] + "十"
	}
	if ones > 0 {
		s += chineseDigits[ones]
	}
	return s
}
