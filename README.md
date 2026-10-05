# Crychic

以 [MoviePilot](https://github.com/jxxghp/MoviePilot) 为后端的聊天机器人，在 Telegram 中搜索影视并创建订阅。交互流程参考 [Doplarr_rs](https://github.com/activexray/doplarr_rs)。

当前范围（MVP）：Telegram、仅订阅、白名单权限。Discord 计划中。

## 使用

```
/request 沙丘
```

选择搜索结果 → （剧集）选择季和起始集 → 确认订阅。已订阅的媒体会提前结束流程。

机器人会通过 MoviePilot 查询媒体服务器（Emby/Jellyfin/Plex）里已有的内容：已在库中的电影和整季不会重复订阅；季按钮会标出「已入库」或「已有 N 集」，部分入库的季在确认时列出已有的集（MoviePilot 订阅只下载缺少的集）。

订阅的内容入库后，机器人会在请求所在的聊天里通知请求人（群聊中会 @ 对方）。

## 配置

全部通过环境变量：

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `CRYCHIC_MOVIEPILOT_URL` | 是 | MoviePilot 地址，如 `http://moviepilot:3000` |
| `CRYCHIC_MOVIEPILOT_API_KEY` | 是 | MoviePilot 设置中的 `API_TOKEN`，以超级管理员身份调用 |
| `CRYCHIC_TELEGRAM_TOKEN` | 是 | BotFather 发放的 Bot Token |
| `CRYCHIC_TELEGRAM_ALLOWED_USERS` | 是 | 允许使用的 Telegram 用户 ID，逗号分隔。陌生人使用时机器人会回复其 ID |
| `CRYCHIC_TELEGRAM_API_URL` | 否 | 自建 Bot API 服务器地址，默认 `https://api.telegram.org` |
| `CRYCHIC_DATA_DIR` | 否 | 数据目录，默认 `data`，保存等待入库通知的请求 |
| `CRYCHIC_NOTIFY_INTERVAL` | 否 | 检查入库的间隔，默认 `1m` |
| `CRYCHIC_NOTIFY_QUIET` | 否 | 剧集入库的静默期，默认 `3m`：一段时间内没有新集入库后，把这段时间到的集合并成一条通知；`0s` 表示不合并 |

```sh
go build -o crychic ./cmd/crychic && ./crychic
```

## 架构

```
cmd/crychic          读取配置，启动
internal/app         组装依赖（main 与 e2e 测试共用）
internal/flow        与平台无关的会话状态机：搜索 → 选择 → 选季 → 确认 → 订阅
internal/moviepilot  精简的 MoviePilot v3 API 客户端（实现 flow.Backend）
internal/telegram    把 flow.Reply 渲染为消息与 Inline Keyboard
internal/notify      记住请求，轮询 MoviePilot 整理记录，入库后通知请求人
internal/config      环境变量配置
```

## 测试

行为优先的端到端测试（参考 [yetone/magpie](https://github.com/yetone/magpie)）：`e2e/` 用与 `main` 相同的组装方式启动真实应用，外部只替换为两个假服务：一个假 Telegram Bot API（真实长轮询 `getUpdates`），一个假 MoviePilot（校验 `X-API-KEY`）。

每个场景把跨越边界的全部交互（用户动作、MoviePilot 请求、Telegram 调用）按顺序记录下来，与 `e2e/testdata/transcripts/` 中已提交的记录逐字比对。这些记录本身就是可审阅、可复现的行为规格。

```sh
go test ./...                    # 运行
go test ./e2e/ -update           # 行为有意变更后重新生成记录，审阅 diff 后提交
```

`e2e/testdata/moviepilot/` 中的响应 fixture 依据 MoviePilot v3.1.0 源码中的响应模型编写，尚未用真实实例录制校准。
