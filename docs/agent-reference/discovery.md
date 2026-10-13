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
name the show 《青之箱》第 1 季), synopsis from TMDB else MoviePilot's `/api/v1/bangumi/{id}`
(`Backend.BangumiSummary`), with direct `/v0/subjects/{id}` (`flow.Calendar.Summary`, using `Media.CalendarID`)
only for still-missing synopses. MoviePilot reads of a round overlap; direct fallbacks are a second parallel
group. Both share one 5-second cosmetic budget per round. A page does not wait for them: `annotate` (notes.go)
starts one background round (`Engine.runNotes`, run context, `Engine.work`) for the page's picks without a fresh
note, waits `Options.NoteGrace` (default 800ms; the e2e harness lets it wait for everything), and renders with what
the engine-level note cache (`notecache.go`, keyed by `CalendarID`, 512 picks, oldest evicted) holds. Each pick is
remembered when its own lookup ends: a found synopsis for 6 hours, a finished lookup that found none (HTTP errors
included, as MoviePilot answers an upstream failure with an empty `MediaInfo`, HTTP 200) for 1 hour; a pick whose
lookup the round's budget cut short is remembered as a miss for 10 minutes (`noteTimeoutTTL`), so views and back/forward
do not fire the same heavy searches again; one cut short by shutdown is not remembered.
All MoviePilot calls of note lookups (title searches and MP Bangumi synopses, every page and round) share
`noteMPSlots` = 2 slots (`Engine.mpSlots`), taken with the round's context so a waiting lookup gives up when the
budget ends; the Bangumi public API fallback is outside the cap. Rationale: MP's media search is CPU-heavy and goes on
after the client cancels, and v0.4.4's week warm plus 8 picks x 3 concurrent calls per page kept MP at 100% CPU
(2026-10-13); the week warm was removed.
Rounds are deduplicated per `CalendarID`, so overlapping pages and presses join one round. A page still waiting
after the grace is shown at once with a Remark 简介补充中… and `Reply.Follow` = `gn` (read-only redraw of the page
from session + cache, never starting a lookup) plus `Reply.Wake` closed when the round ends; the redraw keeps Follow
while a round covering the page is running and drops it when none is. Entries keep their numbers and buttons,
only their link and synopsis appear later. Sessions read the cache while rendering and never take writes from rounds.
A calendar pick opened as a card first takes its cached note, so a known TMDB twin
is searched and matched by identity; its synopsis fills a card whose result has none, and the card's cosmetic reads
then run under `calendarCardTimeout` (2s) instead of 5s.
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
empty; synopses prefer TMDB and MoviePilot, then direct Bangumi within the remaining budget). Bangumi subjects returned by a title search (`/api/v1/media/search`) include manga and novels typed 电视剧 without a year
(2026-10-13, v3.1.4, 冷淡的佐藤同学: anime 634708, manga 312453 `@comic`, novel 296610); opening one made MoviePilot's cold details and
seasons reads take 26–30s and fail. `flow.onlyAnimeAndLive` (`internal/flow/subjects.go`) therefore reads each Bangumi result's
type through `Calendar.SubjectType` (`api.bgm.tv/v0/subjects/{id}`: 1 book, 2 anime, 3 music, 4 game, 6 live-action) and drops those
known to be neither 2 nor 6. Lookups run at once (4 slots) under one 1.5s budget on the request context; an error, timeout or missing
type keeps the result. Known types are cached in the engine forever (LRU, 1024 entries), errors and 404s are not. It applies to typed and
command searches and to the chart-pick fallback list, before the unique-result shortcut, so the list numbering is the filtered list; inline
`Find` (no per-keystroke fan-out) and `Open` (a `Lookup`, not a search) are left alone; calendar note searches keep unfiltered results.
All results dropped reads as no results. The log line `dropped Bangumi results that are not anime` counts them.
A TMDB twin keeps its pick's `Weekday`
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

`internal/flow/seriesplan.go`（第 10 项）：同系列列表在同一 5 秒展示预算内读取一次订阅列表、一次下载列表，并以最多 4 个并发逐部读取媒体库（电影 `notexists`）。状态优先级为已入库、下载中、已订阅／已暂停订阅、未获取；任一决定所需的读取失败或超时即为「状态未知」，只有未获取可勾选。`jq` 前进到勾选页，`jt` 同级切换，`jy` 属于 `finishing` 并绑定当前票据：先消费勾选，再逐部 `FindSubscription` 后最多写一次，已在订阅中的跳过，结果表逐部列出；不持久化库存或完成历史。

相关列表的进入、返回、下载意图清除与空结果 notice 规则见 [会话参考](flow.md#相关推荐)。

## 条件找片与演职人员

2026-10-12 对照 MP `v3.1.4` 源码与前端核验。`/api/v1/discover/tmdb_movies|tmdb_tvs` 接受 `sort_by`、`with_genres`、
`with_original_language`、`vote_average`、`vote_count`、`release_date`、`page`，模块把评分、人数与日期分别作为
`vote_average.gte`、`vote_count.gte`、`release_date.gte`（剧集为 `first_air_date.gte`）传给 TMDB，每页 20 条。题材
id（电影与剧集不同）、13 种语言和排序键取自 MP 官方 TMDB 探索页；评分人数默认 ≥10 与官方一致，避免冷门作品霸占
评分排序。`douban_movies|douban_tvs` 只有 `sort` 与 `tags`，`bangumi` 只有分类、排序与年份，没有评分、人数和日期，
本版不提供它们的条件找片。TMDB 结果自带 TMDB 身份。

`internal/flow/discoverfilter.go`：条件页是一个屏幕，`yf` 在当前页展开某项选项，`yo` 选定后收起，`yk` 切换电影/
电视剧并清空题材；它们都是同级替换，所以「返回」始终回到榜单菜单。`ys` 读取第一页并前进到结果页，`ym` 读取下
一页并按来源与 ID 去重后追加，显示含第一个新结果的那页，旧编号不变；最多 100 条，末页不满 20 条时不再提供
「更多结果」。没有结果只闪提示，条件留在原页。`session.filter` 为值类型，历史快照各自保存条件。

`internal/flow/people.go`：卡片的「演职人员」（`ca`）仅对 TMDB 与 Bangumi 媒体显示。TMDB 读
`/api/v1/tmdb/credits/{id}/{电影|电视剧}`，角色取 `character`；Bangumi 读 `/api/v1/bangumi/credits/{id}`，是番剧配音
人物，`character` 为空，角色取 `career`（角色名）。豆瓣演职员需要另外核验，本版不提供。`cw` 读
`/api/v1/{tmdb|bangumi}/person/credits/{person_id}`：TMDB 按日期倒序，Bangumi 没有年份且同一条目按角色重复出现，
客户端按来源、ID 与类型去重（TMDB 电影与剧集的 ID 各自编号，同号可能是两部作品）。进入作品列表清除前一标题的下载意图，返回恢复原标题状态。作品与条件找片结果都放进 `session.picks`，由 `cp` 走与榜单相同的 `pickFromChart`：TMDB
作品按身份直接对上，其他来源按标题搜索，对不上时说明而不猜测。

## 并发与可恢复元数据

日历注释由引擎级缓存保存（见上，跨会话；找到简介 6 小时，完成但无简介 1 小时），会话渲染前复制可变的 picks slice 并只在渲染时读取缓存。预算到期前未完成的条目不入缓存，下次显示时重新尝试。已取得的结果不会因同页另一个慢条目被丢弃。元数据失败不可抹掉发现列表的返回路径。

并行请求的路由归一化、逐标题假响应与合集候选顺序见 [E2E 参考](e2e.md#并行发现读取)。
