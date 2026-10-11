# 架构与实现入口

首次跨模块定位、修改数据流或寻找既有实现时读取；具体行为分散到各主题参考，不在根文件重复加载。

Crychic 是 Go 媒体订阅与下载机器人，当前主要平台为 Telegram。原指南把 Discord 列为下一平台，这属于计划，不代表已实现。范围仍是订阅、下载与白名单授权。

## 后续开发原则

项目采用「现有 Crychic + 原生 API 优先 + MP 插件补足能力」。保留 Go 应用及既有交互，只为原生 API 的明确缺口开发 MP 侧插件扩展。

| 部分 | 职责 |
| --- | --- |
| 现有 Crychic Go 应用 | 平台接入、白名单、会话、最终确认、展示及安全投递 |
| MP 原生 API | 已能可靠满足需求的查询与操作，继续直接调用 |
| MP 侧 Crychic 插件 | 补足 API 未能提供的能力、事件、上下文或可靠接入机制，复用 MP 原生实现 |

新增功能先核验原生 API 的能力与契约。API 足够时直接使用；存在明确缺口时，在对应功能计划中记录缺口、插件必要性及接入验收条件，再增加所需扩展。插件可以通过小接口供 Go 应用调用或接收结果，不将所有原生请求统一搬进插件。

插件保持薄适配层，优先使用公开 SDK 和受支持的扩展点。不另写搜索、订阅、下载、整理或通知业务引擎，不将整个 Crychic 重写成插件。插件身份与管理员 API key 均不能替代聊天用户授权；白名单、聊天及话题隔离、最终确认和未知结果不重放继续由既有安全边界约束。

后续新增能力默认核对 MP 最新稳定版，保存对应版本与来源。既有版本核验保留用于追溯，不例行重复验证多个旧版本；目标部署有明确版本差异时，单独验证相关差异。

通知接入已增加 MP 侧 Crychic 自定义渠道插件，持久安全投影通过认证长轮询与消费确认传输；Go 继续负责当前富文本展示。详见 [第 6 项前置任务](../plans/moviepilot-api-improvements.md#第-6-项前置任务-moviepilot-原生通知接入)。下文目录及数据流描述当前实现；Discord 仅为后续扩展方向。

## 用户与权限模型

2026-10-11 起按单一信任域运行：白名单内用户同等管理 MP 内全部订阅与下载，包括在 MP 网页端创建的订阅；Crychic 不再维护本地请求归属，也不提供 `/requests`。

原因是 MP 的 API key 只有一个，且固定映射到超级用户（v3.1.4 `app/adapters/web/security/access.py` 的 `verify_token` 与 `validate_api_credential_identity`），MP 无法区分聊天用户。本地归属表只能在 Crychic 内部模拟权限，代价是一整套请求记录、整理历史轮询和完成结算，而且挡住了主人管理在网页端建立的订阅。

多用户隔离后置到 MP 官方支持按用户的 API 凭据之后再设计。保留的扩展接缝：

- `flow.Actor` 仍贯穿每次会话与写入，Telegram 白名单与会话按用户、聊天和话题隔离保持不变。
- 后续按 `Actor` 在 `internal/app` 选择对应的 MP 凭据或 Backend，归属与权限沿用 MP 自身的用户体系，不重建本地归属表。
- 写前的订阅复合身份复核（ID、来源、媒体、季、剧集组、创建时间）与归属无关，继续保留。

旧部署数据目录里的 `requests.json` 与 `requests.json.pre-native` 不再读取，也不删除或改写。

## 目录职责

```text
cmd/crychic/         # main: config.Load(os.Getenv) → app.Run
internal/app/        # the only wiring point; shared by main and e2e
internal/flow/       # platform-agnostic conversation: search → pick (library check) → season → confirm → subscribe
internal/moviepilot/ # minimal MoviePilot client, implements flow.Backend
internal/bangumi/    # Bangumi's public API (no key): airing calendar and synopses, implements flow.Calendar
internal/telegram/   # renders flow.Reply as messages + inline keyboards; whitelist check; delivers notices
internal/notify/     # consumes MP native notices from the Crychic plugin and hands them to the platform
internal/config/     # env vars → config.Config
e2e/                 # behavior tests: real app vs fake Telegram Bot API + fake MoviePilot + fake Bangumi
```

## 数据流与边界

Data flow: Telegram update → `telegram.adapter` → `flow.Engine.Start/Choose` → `flow.Backend` (MoviePilot) →
`flow.Reply` → `sendRichMessage` (command) or `editMessageText` with `rich_message` (button) →
`answerCallbackQuery` last.

平台不得被 flow 导入；具体依赖只在 app 装配，main 与 E2E 共用入口。平台只负责输入、渲染与投递；通知由 MP 原生渠道产生。

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
| 单订阅搜索与跟踪 | [internal/flow/subsearch.go](../../internal/flow/subsearch.go) | `subsearchmodel.go` 保存本会话发起记录，`subsearchread.go` 只读跟踪与停止预检，`subsearchtext.go` 区分确认和执行事实；MoviePilot `subsearch.go`、`subsearchbatch.go` |
| 季与升级 | [internal/flow/seasons.go](../../internal/flow/seasons.go) | `episodes.go`、`upgrade.go`；多季、起始集与洗版 |
| 资源与下载 | [internal/flow/torrents.go](../../internal/flow/torrents.go) | `torrenttext.go`、`torrenthunt.go`、`torrentsort.go`、`torrentmissing.go`、`torrentbatch.go` |
| 缺集下载方案 | [internal/flow/torrentplan.go](../../internal/flow/torrentplan.go) | `torrentplanread.go` 读取及预检，`torrentplanselect.go` 确定性建议，`torrentplanmodel.go` 汇总范围，`torrentplantext.go` 有界预览；确认复用原串行批量下载 |
| 任务与删除 | [internal/flow/tasks.go](../../internal/flow/tasks.go) | `taskdelete.go`、`tasktext.go`；任务跟随与删除确认 |
| 语义文本 | [internal/flow/rich.go](../../internal/flow/rich.go) | `text.go`；语义块、卡片与列表文案 |
| Inline | [internal/flow/inline.go](../../internal/flow/inline.go) | `internal/telegram/inline.go`；无会话查找、经典 HTML 与 deep link |
| 平台输入与时序 | [internal/telegram/input.go](../../internal/telegram/input.go) | `address.go`、`lanes.go`、`follow.go`；引用、topic、锁与重投 |
| 平台渲染与海报 | [internal/telegram/rich.go](../../internal/telegram/rich.go) | `home.go`、`posters.go`；rich HTML、首页 JPEG 与缓存 |
| MoviePilot | [internal/moviepilot/client.go](../../internal/moviepilot/client.go) | `subscriptions.go`、`downloads.go`、`labels.go`、`episodename.go` 等；接口与解析 |
| Bangumi | [internal/bangumi/client.go](../../internal/bangumi/client.go) | 保留星期分组的日历与 subject 简介 |
| 通知 | [internal/notify/native.go](../../internal/notify/native.go) | `native_state.go`、`native_model.go`、`native_text.go`；MoviePilot `native.go` 长轮询与确认 |
| E2E | [e2e/harness_test.go](../../e2e/harness_test.go) | `transcript_test.go`、`fake_*.go` 与 `testdata/transcripts/*.txt` |

## 按主题继续读取

[会话、导航与文案](flow.md)；[MoviePilot 协议](moviepilot.md)；[Telegram
投递](telegram.md)；[通知状态](notifications.md)；[发现与日历](discovery.md)；[E2E 与录制](e2e.md)；[运行配置](operations.md)。

文件名和常量用于定位，不替代实际源码。旧指南中的重复机制已集中到对应主题；原文与新位置的对应关系见 [迁移账目](tools.md#迁移账目)。
