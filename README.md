# Crychic

以 [MoviePilot](https://github.com/jxxghp/MoviePilot) 为后端的聊天机器人，在 Telegram 中搜索影视并创建订阅。交互流程参考 [Doplarr_rs](https://github.com/activexray/doplarr_rs)。

当前范围（MVP）：Telegram、仅订阅、白名单权限。Discord 计划中。

## 使用

私聊直接发送片名即可搜索。`/start` 打开首页，所有功能都有入口；也可以直接用命令：

```
/search 沙丘
```

正在等待起始集数时，普通文字优先作为回答；用 `/search 新片名` 或 `/start` 可以切换流程。群聊仍通过命令操作，输入回答时需要引用机器人的问题。

选择搜索结果 → （剧集）选择季和起始集 → 确认订阅。已订阅的媒体会提前结束流程。剧集也可以点「多选季…」一次勾选多季，每季从第 1 集开始订阅，已在订阅中的季会跳过（仍会通知）。

搜索结果每页最多 8 条，可翻页查看全部返回结果；选片后返回会恢复原来的页。只有一个结果时直接打开媒体卡片，只有一个常规季且没有特别篇时省去选季，但最终订阅仍需确认。点「重新搜索」可以在原消息中换片，旧查询的按钮随之失效；没有结果时可以直接回复新的片名。

搜索、季信息或已有订阅查询失败时，可以在原消息中点「重试」，保留已经选好的媒体和季；也可以重新搜索、返回上一步或取消。重试只重复失败的查询，不会创建订阅。等待输入的请求过期后，当前回答只提示失效，下一条片名可正常发起搜索。

订阅被 MoviePilot 明确拒绝时会显示原因。提交后遇到网络或响应异常，或者没有收到有效的订阅 ID，会提示先用 `/subscribe` 核对结果，不自动重发；旧确认按钮也不能再次提交。

机器人会通过 MoviePilot 查询媒体服务器（Emby/Jellyfin/Plex）里已有的内容：已在库中的电影和整季不会重复订阅；季按钮会标出「已入库」或「已有 N 集」，部分入库的季在确认时列出已有的集（MoviePilot 订阅只下载缺少的集）。选定后还会显示下载器里对应任务的进度。

媒体库查询失败时，卡片会显示「媒体库状态暂时无法确认」，仍可手动确认订阅。详情、海报和下载进度不可用时也不会阻断订阅流程。

订阅的内容入库后，机器人会在请求所在的聊天里通知请求人（群聊中会 @ 对方）。通知会等到媒体服务器（Emby 等）能看到新文件再发（最多等 `CRYCHIC_NOTIFY_LIBRARY_WAIT`），并附上「▶️ 在 Emby 中观看」链接。

```
/newly_added
```

媒体库最新入库的内容，点片名直接打开媒体服务器网页观看。

```
/trending
```

浏览 TMDB 流行趋势、豆瓣热门电影/剧集、正在热映和新番放送表（按周几分页，默认今天）。选中一部会自动找到对应的 TMDB 条目进入订阅流程；豆瓣/Bangumi 条目对不上时让你在搜索结果里挑。

```
/subscribe
```

列出 MoviePilot 的所有订阅（状态、缺几集）；你通过 Crychic 请求的订阅可以在这里取消，取消后不再通知。

```
/tasks
```

列出 MoviePilot 下载中和整理中的任务（只含 MoviePilot 添加的下载）。选中一个任务后，消息会自动刷新进度，直到任务结束、按「停止刷新」或刷新满 10 分钟（可「继续刷新」）；任务页都有「返回任务列表」。整理任务显示每个文件的状态，没有百分比。

## 配置

全部通过环境变量：

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `CRYCHIC_MOVIEPILOT_URL` | 是 | MoviePilot 地址，如 `http://moviepilot:3000` |
| `CRYCHIC_MOVIEPILOT_API_KEY` | 是 | MoviePilot 设置中的 `API_TOKEN`，以超级管理员身份调用 |
| `CRYCHIC_TELEGRAM_TOKEN` | 是 | BotFather 发放的 Bot Token |
| `CRYCHIC_TELEGRAM_ALLOWED_USERS` | 是 | 允许使用的 Telegram 用户 ID，逗号分隔。陌生人使用时机器人会回复其 ID |
| `CRYCHIC_TELEGRAM_API_URL` | 否 | 自建 Bot API 服务器地址，默认 `https://api.telegram.org` |
| `CRYCHIC_BANGUMI_API_URL` | 否 | Bangumi API 地址，默认 `https://api.bgm.tv`；新番放送按周几读取它的每日放送 |
| `CRYCHIC_DATA_DIR` | 否 | 数据目录，默认 `data`，保存等待入库通知的请求 |
| `CRYCHIC_NOTIFY_INTERVAL` | 否 | 检查入库的间隔，默认 `1m` |
| `CRYCHIC_NOTIFY_LIBRARY_WAIT` | 否 | 入库通知最多等媒体服务器多久，默认 `30m`；`0s` 表示整理完立即通知 |
| `CRYCHIC_PROGRESS_INTERVAL` | 否 | `/tasks` 进度的刷新间隔，默认 `5s` |
| `CRYCHIC_NOTIFY_QUIET` | 否 | 剧集入库的静默期，默认 `3m`：一段时间内没有新集入库后，把这段时间到的集合并成一条通知；`0s` 表示不合并 |

```sh
go build -o crychic ./cmd/crychic && ./crychic
```

## 部署（Docker）

```sh
cp compose.example.yaml compose.yaml   # 填入 API Key、Bot Token、白名单
mkdir -p data && sudo chown 65532:65532 data
docker compose up -d --build
```

镜像基于 `distroless/static`（约 15 MB），以 uid 65532 运行，数据目录挂载到 `/data`。示例使用 host 网络，以便访问同机的 MoviePilot（`127.0.0.1:3001`）。

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

聊天适配器通过共享的 `flow.Conversation` 驱动会话，通过 `notify.Sender` 投递入库通知；平台负责输入和渲染，业务流程不依赖 Telegram。Discord 尚未实现。

## 测试

行为优先的端到端测试（参考 [yetone/magpie](https://github.com/yetone/magpie)）：`e2e/` 用与 `main` 相同的组装方式启动真实应用，外部只替换为两个假服务：一个假 Telegram Bot API（真实长轮询 `getUpdates`），一个假 MoviePilot（校验 `X-API-KEY`）。

每个场景把跨越边界的全部交互（用户动作、MoviePilot 请求、Telegram 调用）按顺序记录下来，与 `e2e/testdata/transcripts/` 中已提交的记录逐字比对。这些记录本身就是可审阅、可复现的行为规格。

```sh
go test ./...                    # 运行
go test ./e2e/ -update           # 行为有意变更后重新生成记录，审阅 diff 后提交
```

`e2e/testdata/moviepilot/` 中的响应 fixture 依据 MoviePilot v3.1.0 源码中的响应模型编写，尚未用真实实例录制校准。
