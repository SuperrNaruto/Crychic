package flow

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

const (
	upcomingDays = 7
	// upcomingReads bounds the season reads running at once.
	upcomingReads = 4
	dateLayout    = "2006-01-02"

	msgUpcomingTitle = "📅 追剧日历"
	msgUpcomingNote  = "接下来 7 天（北京时间），点编号看订阅详情"
	msgNoUpcoming    = "接下来 7 天你订阅的剧都没有新集播出哦 (´-ω-`)"
	msgNoUpcomingTip = "接下来 7 天没有你订阅的剧播出哦～"
)

// airing is one episode airing soon, of the subscription at sub.
type airing struct {
	sub  int
	date time.Time
	ep   EpisodeAir
}

// airings is what the calendar found, and how many TV subscriptions it
// could not read (failed) or cannot read at all (skipped: not on TMDB).
type airings struct {
	today           time.Time
	list            []airing
	failed, skipped int
}

// Upcoming opens the calendar of the subscribed shows' coming episodes.
func (e *Engine) Upcoming(ctx context.Context, actor Actor) Reply {
	sess := e.store.create(actor, nil)
	return e.shown(sess.id, e.upcoming(ctx, sess))
}

// upcoming lists the episodes of subscribed seasons airing in the next
// upcomingDays days, grouped by day; number buttons open the subscription.
func (e *Engine) upcoming(ctx context.Context, sess session) Reply {
	subs, err := e.backend.Subscriptions(ctx)
	if err != nil {
		e.store.take(sess.id)
		return titled(msgUpcomingTitle, e.failure("subscriptions", err))
	}
	sess.subs, sess.mine = subs, e.watcher.Requested(sess.owner.UserID)
	found := e.readAirings(ctx, subs, e.now().In(calendarZone))
	if len(found.list) == 0 && found.failed == 0 && sess.menu {
		return Reply{Notice: msgNoUpcomingTip}
	}
	e.store.put(sess)
	if len(found.list) == 0 {
		text := Lines(Heading(Plain(msgUpcomingTitle)), Line(Plain(msgNoUpcoming)))
		if note := found.gaps(); note != "" {
			text = append(text, Remark(note+"～"))
		}
		return Reply{Text: text, Buttons: [][]Button{browseRow(sess.id)}}
	}
	return e.listPages(sess, upcomingView(sess, found))
}

// readAirings reads the seasons of the TV subscriptions a few at a time and
// keeps the episodes airing from today on for upcomingDays days.
func (e *Engine) readAirings(ctx context.Context, subs []Subscription, now time.Time) airings {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, calendarZone)
	seasons := make([][]EpisodeAir, len(subs))
	errs := make([]error, len(subs))
	found := airings{today: today}
	var wg sync.WaitGroup
	slots := make(chan struct{}, upcomingReads)
	for i, s := range subs {
		if s.Kind != TV {
			continue
		}
		if s.Source != tmdbSource || s.Season == nil {
			found.skipped++
			continue
		}
		media := Media{Source: s.Source, ID: s.MediaID, Title: s.Title, Year: s.Year, Kind: TV}
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			seasons[i], errs[i] = e.backend.SeasonEpisodes(ctx, media, *s.Season)
		})
	}
	wg.Wait()
	for i, eps := range seasons {
		if errs[i] != nil {
			e.log.Warn("season episodes unavailable", "subscription", subs[i].ID, "err", errs[i])
			found.failed++
		}
		found.list = append(found.list, airingSoon(i, eps, today)...)
	}
	slices.SortStableFunc(found.list, func(a, b airing) int {
		return cmp.Or(a.date.Compare(b.date), cmp.Compare(subs[a.sub].Title, subs[b.sub].Title), cmp.Compare(a.ep.Number, b.ep.Number))
	})
	return found
}

// airingSoon are the episodes of the subscription at sub airing within
// upcomingDays days of today.
func airingSoon(sub int, eps []EpisodeAir, today time.Time) []airing {
	var soon []airing
	for _, ep := range eps {
		date, err := time.ParseInLocation(dateLayout, ep.Date, calendarZone)
		if err != nil || date.Before(today) || !date.Before(today.AddDate(0, 0, upcomingDays)) {
			continue
		}
		soon = append(soon, airing{sub: sub, date: date, ep: ep})
	}
	return soon
}

// gaps says which shows the calendar could not cover, "" when none.
func (a airings) gaps() string {
	var notes []string
	if a.failed > 0 {
		notes = append(notes, fmt.Sprintf("有 %d 部剧暂时没读到播出时间", a.failed))
	}
	if a.skipped > 0 {
		notes = append(notes, fmt.Sprintf("%d 部不是 TMDB 条目，查不到播出时间", a.skipped))
	}
	return joinNonEmpty("；", notes...)
}

// upcomingView lists the airings one day an entry, so a day never splits
// across pages, numbered across days.
func upcomingView(sess session, found airings) listView {
	view := listView{
		heading: Heading(Plain(msgUpcomingTitle)),
		note:    joinNonEmpty("；", msgUpcomingNote, found.gaps()) + "～",
		footer:  browseRow(sess.id),
	}
	n := 0
	for start := 0; start < len(found.list); {
		end := start
		day := listEntry{text: Lines(Group(dayLabel(found.list[start].date, found.today)))}
		for ; end < len(found.list) && found.list[end].date.Equal(found.list[start].date); end++ {
			n++
			day.text = append(day.text, airingEntry(sess, n, found.list[end]))
			day.buttons = append(day.buttons, Button{Label: fmt.Sprint(n), Data: data(sess.id, actionSubDetail, found.list[end].sub)})
		}
		view.entries = append(view.entries, day)
		start = end
	}
	return view
}

// airingEntry is e.g. "3. **择日飞升**" over "_第 1 季 第 15 集_", marked
// 已暂停 when its subscription will not fetch it.
func airingEntry(sess session, n int, a airing) Block {
	s := sess.subs[a.sub]
	name := a.ep.Name
	if name == fmt.Sprintf("第 %d 集", a.ep.Number) {
		name = ""
	}
	paused := ""
	if s.State == statePaused {
		paused = stateText[statePaused]
	}
	item := entry(n, Strong(truncate(s.Title, listTitleRunes)),
		fmt.Sprintf("%s 第 %d 集", seasonName(*s.Season), a.ep.Number), name, paused)
	if slices.Contains(sess.mine, s.ID) {
		item.Tag = msgRequestedTag
	}
	return item
}

// dayLabel is e.g. "10月7日 周三 · 今天".
func dayLabel(date, today time.Time) string {
	label := fmt.Sprintf("%d月%d日 周%s", date.Month(), date.Day(), weekdayNames[(int(date.Weekday())+daysInWeek-1)%daysInWeek])
	switch {
	case date.Equal(today):
		label += " · 今天"
	case date.Equal(today.AddDate(0, 0, 1)):
		label += " · 明天"
	}
	return label
}
