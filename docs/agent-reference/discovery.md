# 日历、图表与媒体身份参考

改 Bangumi/TMDB 日历、图表身份、简介或同系列查询时读取；涉及 flow 与外部客户端的规则要一起读。

本文从原根指南按主题迁出，保留当时的实现说明与上游版本背景，不代表已对当前线上服务重新核验。涉及目标版本变化时，按根指南核验来源。

## 星期与日历注释

`internal/flow/charts.go` calendar: 新番放送 opens on today's weekday (China time, `calendarZone`), one weekday
per view with a 一…日 button row, picks sorted by first air date within the day

`internal/flow/notes.go` - `/trending` calendar pages: each pick looked up on TMDB (title, then original
title) for link + identity (same year and kind; a sequel whose search term names a later season, MoviePilot's
`season`, may start in an earlier year, since TMDB dates a show by its first season: 青之箱 第二季 is TMDB 207347 of
2024, so its subscription matches transfers; the pick keeps its own name, and the 第X季 MoviePilot's search
appends to a TMDB show's title, `flow.SeasonSuffix`, is stripped by `moviepilot` so cards and subscriptions
name the show 《青之箱》第 1 季), synopsis from TMDB else Bangumi's own `/v0/subjects/{id}` (`flow.Calendar.Summary`,
kept on `Media.CalendarID`); all three lookups of every pick on a page run at once (one round trip); a pick
found with a synopsis is done for the session, one without is looked up again whenever its page shows
(MoviePilot answers an upstream failure with an empty `MediaInfo`, HTTP 200, which is why synopses skip it).
Other charts show the synopsis the chart carries (Douban's is a region / genre / director / cast line); chart
and related entries are `Brief` (facts on the name's line, ` · ` apart); a chart entry with a synopsis opens
by its name (`Block.Body`: synopsis, then 查看详情页 link), one without has its name linked

## 跨来源身份与日历数据源

Chart picks (`/api/v1/recommend/*`) are search-shaped but Douban/Bangumi ones carry Douban/Bangumi ids, while
transfer history only carries TMDB ids; a pick is therefore searched by title (its original title searched at
the same time, used only when the title finds nothing) and only taken directly when the identity or a unique
title+year+kind matches, else the user picks. Douban season titles like 「流人 第六季」 often find nothing right.
MoviePilot's Bangumi calendar (`/api/v1/recommend/bangumi_calendar`) loses the weekday grouping and serves 30
items a page (Monday to Wednesday), so the bot reads `api.bgm.tv/calendar` itself (summaries there are always
empty; synopses come from TMDB, else Bangumi's subject API directly). A TMDB twin keeps its pick's `Weekday`
and air date.

## 追剧日历

`internal/flow/upcoming.go` - `/upcoming` (home 追剧日历): the TV subscriptions' seasons (TMDB only; others
counted as skipped) read a few at once (`upcomingReads`) from `GET /api/v1/tmdb/{tmdbid}/{season}`
(`Backend.SeasonEpisodes`, `air_date` a civil date; a subscription's custom 剧集组 `episode_group` is passed on,
as MoviePilot numbers its seasons in that group; the fake answers it as `episodeGroupSeason`), episodes airing
today through 6 days on in China time, one list entry per day (a `Group` date, so a day never splits across
pages) numbered across days; numbers open the subscription's details (`sd`, forward, so 返回 shows the calendar
even if the subscription has since ended); a paused subscription's episodes say 已暂停; a failed season read only
adds a footer note; nothing airing flashes a notice from home, answers a message to the command. TMDB names
unknown episodes 第 N 集, which is not repeated

## 相关推荐与同系列

相似推荐 reads `GET /api/v1/tmdb/recommend/{tmdbid}/{电影|电视剧}` (TMDB media only; `type_name` must be the Chinese
enum, `movie` answers 未知错误); `/tmdb/similar` is not used, its picks are poor. MoviePilot's details leave
`collection_id` null (it reads TMDB's `belongs_to_collection` under the wrong name and drops `tmdb_info`), so
同系列 searches collections (`/api/v1/media/search?type=collection`, items `type` 系列) by the movie's titles and
their stems (沙丘2 → 沙丘, `Dune: Part Two` → `Dune`), searches every name at once, then opens up to 3 candidates
per name at once (`/api/v1/tmdb/collection/{id}`), and takes the first listing the movie in name order (a
failed read before it is the answer); a series named unlike its films (哈利·波特与魔法石) is not found.

相关列表的进入、返回、下载意图清除与空结果 notice 规则见 [会话参考](flow.md#相关推荐)。

## 并发与可恢复元数据

日历注释写入前复制两个可变 slice。找到简介的条目在当前会话中缓存；仍无简介的条目再次显示时重新尝试。元数据失败不可抹掉发现列表的返回路径。

并行请求的路由归一化、逐标题假响应与合集候选顺序见 [E2E 参考](e2e.md#并行发现读取)。
