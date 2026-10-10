# 架构与实现入口

首次跨模块定位、修改数据流或寻找既有实现时读取；具体行为分散到各主题参考，不在根文件重复加载。

Crychic 是 Go 媒体订阅与下载机器人，当前主要平台为 Telegram。原指南把 Discord 列为下一平台，这属于计划，不代表已实现。范围仍是订阅、下载与白名单授权。

## 目录职责

```text
cmd/crychic/         # main: config.Load(os.Getenv) → app.Run
internal/app/        # the only wiring point; shared by main and e2e
internal/flow/       # platform-agnostic conversation: search → pick (library check) → season → confirm → subscribe
internal/moviepilot/ # minimal MoviePilot client, implements flow.Backend
internal/bangumi/    # Bangumi's public API (no key): airing calendar and synopses, implements flow.Calendar
internal/telegram/   # renders flow.Reply as messages + inline keyboards; whitelist check; delivers notices
internal/notify/     # remembers requests (data dir JSON), polls MoviePilot transfer history, notifies requesters
internal/config/     # env vars → config.Config
e2e/                 # behavior tests: real app vs fake Telegram Bot API + fake MoviePilot + fake Bangumi
```

## 数据流与边界

Data flow: Telegram update → `telegram.adapter` → `flow.Engine.Start/Choose` → `flow.Backend` (MoviePilot) →
`flow.Reply` → `sendRichMessage` (command) or `editMessageText` with `rich_message` (button) →
`answerCallbackQuery` last.

平台不得被 flow 导入；具体依赖只在 app 装配，main 与 E2E 共用入口。通知请求通过 watcher 保存，平台只负责输入、渲染与投递。

## 关键文件索引

| 主题 | 入口 | 邻近实现 |
| --- | --- | --- |
| 共享边界 | [internal/flow/model.go](../../internal/flow/model.go) | `Backend`、`Reply`、`UserError`；`conversation.go` 是适配器消费的会话入口 |
| 搜索与恢复 | [internal/flow/search.go](../../internal/flow/search.go) | `engine.go`、`intent.go`、`recovery.go`；搜索意图、只读恢复 |
| 导航与会话 | [internal/flow/nav.go](../../internal/flow/nav.go) | `store.go`、`locking.go`、`pages.go`；快照、票据、TTL 与分页 |
| 首页与最新入库 | [internal/flow/home.go](../../internal/flow/home.go) | `latest.go`；空菜单提示与标题输入 |
| 发现与关联 | [internal/flow/charts.go](../../internal/flow/charts.go) | `notes.go`、`related.go`、`upcoming.go`；日历、简介、合集与追剧日期 |
| 订阅与历史 | [internal/flow/subs.go](../../internal/flow/subs.go) | `subdetail.go`、`subdetailtext.go`、`subprogress.go`、`subpause.go`、`subidentity.go`、`history.go` |
| 订阅条件编辑 | [internal/flow/subedit.go](../../internal/flow/subedit.go) | `subeditchoices.go`、`subeditdirectory.go`、`subeditinput.go`、`subeditsave.go`；`subeditleave.go` 保护草稿，`subeditcheck.go` 核对只读回执，`subeditconflict.go` 处理冲突；MoviePilot `subedit.go`、`subrules.go` |
| 季与升级 | [internal/flow/seasons.go](../../internal/flow/seasons.go) | `episodes.go`、`upgrade.go`；多季、起始集与洗版 |
| 资源与下载 | [internal/flow/torrents.go](../../internal/flow/torrents.go) | `torrenttext.go`、`torrenthunt.go`、`torrentsort.go`、`torrentmissing.go`、`torrentbatch.go` |
| 任务与删除 | [internal/flow/tasks.go](../../internal/flow/tasks.go) | `taskdelete.go`、`tasktext.go`；任务跟随与删除确认 |
| 我的请求与异常 | [internal/flow/requests.go](../../internal/flow/requests.go) | `requestreads.go`、`requestidentity.go`、`requestissues.go`、`requesttext.go`；MoviePilot `failures.go` 读取失败历史，notify `requests.go` 投影归属与保存最近完成 |
| 语义文本 | [internal/flow/rich.go](../../internal/flow/rich.go) | `text.go`；语义块、卡片与列表文案 |
| Inline | [internal/flow/inline.go](../../internal/flow/inline.go) | `internal/telegram/inline.go`；无会话查找、经典 HTML 与 deep link |
| 平台输入与时序 | [internal/telegram/input.go](../../internal/telegram/input.go) | `address.go`、`lanes.go`、`follow.go`；引用、topic、锁与重投 |
| 平台渲染与海报 | [internal/telegram/rich.go](../../internal/telegram/rich.go) | `home.go`、`posters.go`；rich HTML、首页 JPEG 与缓存 |
| MoviePilot | [internal/moviepilot/client.go](../../internal/moviepilot/client.go) | `subscriptions.go`、`downloads.go`、`transfers.go`、`episodename.go` 等；接口与解析 |
| Bangumi | [internal/bangumi/client.go](../../internal/bangumi/client.go) | 保留星期分组的日历与 subject 简介 |
| 通知 | [internal/notify/notifier.go](../../internal/notify/notifier.go) | `state.go`、`cursor.go`、`digest.go`、`stall.go`、`subscriptions.go`、`quality.go` |
| E2E | [e2e/harness_test.go](../../e2e/harness_test.go) | `transcript_test.go`、`fake_*.go` 与 `testdata/transcripts/*.txt` |

## 按主题继续读取

[会话、导航与文案](flow.md)；[MoviePilot 协议](moviepilot.md)；[Telegram
投递](telegram.md)；[通知状态](notifications.md)；[发现与日历](discovery.md)；[E2E 与录制](e2e.md)；[运行配置](operations.md)。

文件名和常量用于定位，不替代实际源码。旧指南中的重复机制已集中到对应主题；原文与新位置的对应关系见 [迁移账目](tools.md#迁移账目)。
