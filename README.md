# Crychic

以 [MoviePilot](https://github.com/jxxghp/MoviePilot) 为后端的聊天机器人，在 Telegram 中搜索影视并创建订阅。交互流程参考 [Doplarr_rs](https://github.com/activexray/doplarr_rs)。

当前范围：Telegram、订阅与选种下载、白名单权限。Discord 计划中。

## 使用

私聊直接发送片名即可搜索。`/start` 打开首页，所有功能都有入口；也可以直接用命令：

```
/search 沙丘
```

正在等待起始集数时，普通文字优先作为回答；用 `/search 新片名` 或 `/start` 可以切换流程。群聊仍通过命令操作，输入回答时需要引用机器人的问题。

所有页面的按钮遵循同一套约定：最后一行是导航。「返回」回到上一屏；「取消」结束正在进行的搜索、订阅或下载；浏览类页面（订阅、任务、榜单、最新入库）用「首页」回到首页、「关闭」收起消息。删除、取消订阅这类不可撤销的操作会先单独确认。

选择搜索结果 → （剧集）选择季和起始集 → 确认订阅。已订阅的媒体会提前结束流程。剧集也可以点「多选季…」一次勾选多季，每季从第 1 集开始订阅，已在订阅中的季会跳过（仍会通知）。

搜索结果每页最多 8 条，可翻页查看全部返回结果；选片后返回会恢复原来的页。只有一个结果时直接打开媒体卡片，只有一个常规季且没有特别篇时省去选季，但最终订阅仍需确认。点「重新搜索」可以在原消息中换片，旧查询的按钮随之失效；没有结果时可以直接回复新的片名。

搜索、季信息或已有订阅查询失败时，可以在原消息中点「重试」，保留已经选好的媒体和季；也可以重新搜索、返回上一步或取消。重试只重复失败的查询，不会创建订阅。等待输入的请求过期后，当前回答只提示失效，下一条片名可正常发起搜索。

订阅被 MoviePilot 明确拒绝时会显示原因。提交后遇到网络或响应异常，或者没有收到有效的订阅 ID，会提示先用 `/subscribe` 核对结果，不自动重发；旧确认按钮也不能再次提交。

机器人会通过 MoviePilot 查询媒体服务器（Emby/Jellyfin/Plex）里已有的内容：已在库中的电影和整季不会重复订阅；季按钮会标出「已入库」或「已有 N 集」，部分入库的季在确认时列出已有的集（MoviePilot 订阅只下载缺少的集）。选定后还会显示下载器里对应任务的进度。

媒体库查询失败时，卡片会显示「媒体库状态暂时无法确认」，仍可手动确认订阅。详情、海报和下载进度不可用时也不会阻断订阅流程。

选定电影或某一季后，卡片上有「搜索资源」：MoviePilot 按 TMDB 身份（和季）去各个站点搜种，和 WebUI 的「搜索资源」相同，通常要几十秒，消息会显示已经搜了多少秒，搜索期间可以随时「取消」。结果默认按 MoviePilot 的优先级排列，也可以像 WebUI 一样改按做种数、发布时间、大小或站点排序，用「筛选站点」只看某个站点的资源（都在已有结果里切换，不会重新搜索）；每页 8 条，显示种子标题、站点、大小、做种数、促销和 H&R；点编号看详情（分辨率、版本、编码、制作组），「确认下载」把这个资源交给 MoviePilot 的下载器。站点 Cookie 和下载链接只在机器人内存里转交，不会显示或写进日志。下载被拒绝时显示原因；结果不确定时提示先用 `/tasks` 核对，不会重复提交。下载的文件入库后同样会通知请求人，下载长时间没有进展也会提醒一次。

订阅的内容入库后，机器人默认在请求所在的聊天里通知请求人（群聊中会 @ 对方）。设置 `CRYCHIC_TELEGRAM_NOTIFY_CHAT_ID` 后，通知改为只发到指定频道，不再发回请求聊天，也不公开请求人的身份；多人请求同一条订阅时，频道只收到一条通知。它仍然只通知通过 Crychic 请求的内容，不广播整个媒体库。

通知会等到媒体服务器（Emby 等）能看到新文件再发（最多等 `CRYCHIC_NOTIFY_LIBRARY_WAIT`），并在能匹配到观看地址时附上「在 Emby 中观看」链接。

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

按电视剧、电影分开列出 MoviePilot 的所有订阅（状态、缺几集），用按钮切换。你通过 Crychic 请求的订阅可以用「取消订阅」选出来取消，取消后不再通知。已结束的订阅在「订阅历史」里，每类显示最近 10 条，可以按原来的设置重新订阅。

点编号打开只读的订阅详情，查看起始集、规格、规则组，以及最近一次搜索的状态和时间（北京时间）。「刷新」重新查询，「返回」恢复原来的订阅列表页，不会触发补搜、下载或修改订阅。

详情把媒体库状态与下载、整理进度分列展示，相同状态的连续集数合并成一行。下载百分比是关联任务的整体进度；整理完成不等于媒体服务器已经可见，MoviePilot 的待获取集数也不等于尚未入库的集数。查询失败显示未知；没有执行记录时不会猜测原因。每次最多展示请求范围内的前 100 集，每页最多 20 行，超出部分明确提示。详情不展示资源名、站点、下载器或文件路径，后台执行错误只提示到 MoviePilot 查看。

```
/tasks
```

列出 MoviePilot 下载中和整理中的任务（只含 MoviePilot 添加的下载）。选中一个任务后，消息会自动刷新进度，直到任务结束、按「停止刷新」或刷新满 10 分钟（可「继续刷新」）；任务页的「返回」会重新读取任务列表。整理任务显示每个文件的状态，没有百分比。

下载任务页有「删除任务」，确认后 MoviePilot 会删除任务**和已下载的文件**。如果同一部片（同一季）还有订阅，确认页会提醒：订阅下的任务删掉后，MoviePilot 已把这些集记为已获取，订阅不会自动重新下载。删除后不再通知这个下载的入库。

## 配置

全部通过环境变量：

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `CRYCHIC_MOVIEPILOT_URL` | 是 | MoviePilot 地址，如 `http://moviepilot:3000` |
| `CRYCHIC_MOVIEPILOT_API_KEY` | 是 | MoviePilot 设置中的 `API_TOKEN`，以超级管理员身份调用 |
| `CRYCHIC_TELEGRAM_TOKEN` | 是 | BotFather 发放的 Bot Token |
| `CRYCHIC_TELEGRAM_ALLOWED_USERS` | 是 | 允许使用的 Telegram 用户 ID，逗号分隔。陌生人使用时机器人会回复其 ID |
| `CRYCHIC_TELEGRAM_API_URL` | 否 | 自建 Bot API 服务器地址，默认 `https://api.telegram.org` |
| `CRYCHIC_TELEGRAM_NOTIFY_CHAT_ID` | 否 | 入库通知频道，如 `-1001234567890` 或 `@crychic_arrivals`；留空保持原聊天通知 |
| `CRYCHIC_BANGUMI_API_URL` | 否 | Bangumi API 地址，默认 `https://api.bgm.tv`；新番放送按周几读取它的每日放送 |
| `CRYCHIC_DATA_DIR` | 否 | 数据目录，默认 `data`，保存等待入库通知的请求 |
| `CRYCHIC_NOTIFY_INTERVAL` | 否 | 检查入库的间隔，默认 `1m` |
| `CRYCHIC_NOTIFY_LIBRARY_WAIT` | 否 | 入库通知最多等媒体服务器多久，默认 `30m`；`0s` 表示整理完立即通知 |
| `CRYCHIC_PROGRESS_INTERVAL` | 否 | `/tasks` 进度的刷新间隔，默认 `5s` |
| `CRYCHIC_NOTIFY_QUIET` | 否 | 剧集入库的静默期，默认 `3m`：一段时间内没有新集入库后，把这段时间到的集合并成一条通知；`0s` 表示不合并 |
| `CRYCHIC_NOTIFY_STALL` | 否 | 下载卡住提醒，默认 `6h`：你请求的剧集或电影对应的下载这么久进度没有变化（手动暂停的不算），就提醒一次（设置了通知频道则发到频道）；每个种子只提醒一次；`0s` 关闭。MoviePilot 只要加了种子就算订阅完成、不再检查进度，所以死种不提醒就会一直等不到入库 |

```sh
go build -o crychic ./cmd/crychic && ./crychic
```

### 独立通知频道

把机器人添加为目标频道的管理员，并授予发布消息的权限。公开频道可以填 `@频道用户名`，私有频道使用数字频道 ID。

```sh
CRYCHIC_TELEGRAM_NOTIFY_CHAT_ID=-1001234567890
```

此配置只改变入库通知的投递位置；订阅等交互继续在与机器人的聊天中进行。频道成员可以看到通知中的片名、集数及媒体服务器观看链接，请按频道受众设置访问权限。频道不可写时会记录投递错误，不回退到请求聊天；通知状态仍按原有策略先保存再发送，发送失败不会自动补发。

频道 `chat_id` 格式和发布权限依据 [Telegram Bot API](https://core.telegram.org/bots/api#sendrichmessage)。

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

`e2e/testdata/moviepilot/` 大部分 fixture 是 MoviePilot v3.1.0 的裁剪录制；不能安全触发的写入结果按同版本源码编写。
