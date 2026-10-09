# 运行、配置与检查参考

执行构建、专项检查或修改配置、启动和部署时读取。源码是当前默认值及验证逻辑的事实来源；本页保留原指南的完整操作索引。

构建与检查命令不会授权真实 MoviePilot 写入或站点搜索。仅文档改动检查文本、链接与 diff；不为此更新 golden 或运行会触发真实业务的机器人。

## 命令

| Command | Description |
|---------|-------------|
| `go build ./cmd/crychic` | Build the binary |
| `go vet ./... && gofmt -l .` | Lint; both must print nothing |
| `go test -race ./...` | Run all tests (they live only in `e2e/`) |
| `go test ./e2e/ -run TestSubscribeMovie` | Run one scenario |
| `docker build -t crychic .` | Build the image (distroless, uid 65532, data at `/data`); `compose.example.yaml` shows a deployment |
| `go test ./e2e/ -update` | Regenerate golden transcripts after an intended behavior change |

## 检查时机

Go 改动执行根指南的 build、vet、gofmt 检查与 race E2E；容器改动再核对 Docker 构建。`go vet` 和 `gofmt -l` 均须无输出，退出码与输出一起判断。

有意改变交互行为才用 `go test ./e2e/ -update`，之后逐行审阅 transcript。普通测试不会重写 golden。失败或不能执行的检查如实报告，不能为提交绕过。

## 环境变量

- `CRYCHIC_MOVIEPILOT_URL`, `CRYCHIC_MOVIEPILOT_API_KEY` (MoviePilot `API_TOKEN`), `CRYCHIC_TELEGRAM_TOKEN` - required
- `CRYCHIC_MOVIEPILOT_COOKIE` - optional resource Cookie value; enables non-Agent resource-search SSE, empty keeps JSON search. No Cookie header, automatic login or renewal. Public DNS names require HTTPS; private/loopback IPs and single-label service names may use HTTP. Never print or persist the value.
- `CRYCHIC_TELEGRAM_ALLOWED_USERS` - required, comma-separated user IDs; empty refuses to start
- `CRYCHIC_TELEGRAM_API_URL` - optional, defaults to `https://api.telegram.org`
- `CRYCHIC_TELEGRAM_NOTIFY_CHAT_ID` - optional arrival channel, negative ID or `@channelusername`; empty notifies request chats
- `CRYCHIC_BANGUMI_API_URL` - optional, defaults to `https://api.bgm.tv`; the 新番放送 calendar is read from it
- `CRYCHIC_DATA_DIR` - optional, defaults to `data`; holds `requests.json` (pending requests, last seen transfer)
- `CRYCHIC_NOTIFY_INTERVAL` - optional, defaults to `1m`, minimum `100ms`; how often transfer history is polled
- `CRYCHIC_NOTIFY_LIBRARY_WAIT` - optional, defaults to `30m`, `0s` disables; how long a settled notice waits for the media server to show the arrival
- `CRYCHIC_PROGRESS_INTERVAL` - optional, defaults to `5s`, minimum `100ms`; task and resource-list refresh cadence. SSE first results and terminal state wake the existing follower early; intermediate frames do not each trigger an edit.
- `CRYCHIC_NOTIFY_STALL` - optional, defaults to `6h`, `0s` disables; how long a requested download may make no progress before its requesters (or the notify channel) are told once that it seems stuck
- `CRYCHIC_NOTIFY_QUIET` - optional, defaults to `3m`, `0s` disables; how long a show's arrivals settle before one notice covers them
- `CRYCHIC_NOTIFY_DIGEST` - optional weekly digest moment in China time, e.g. `sun 20:00`; empty sends none. Goes to the notify channel when set, else privately to each whitelisted user

## 配置与部署入口

配置及最小间隔见 [config.go](../../internal/config/config.go)；公开用法见 [README](../../README.md#配置)，容器定义见
[Dockerfile](../../Dockerfile) 和 [compose 示例](../../compose.example.yaml)。

镜像使用 distroless、uid 65532，持久数据挂载 `/data`。原项目部署示例采用 host 网络；只有首页 banner 随二进制嵌入，不能误把缺媒体资源当可忽略配置。

通知频道是对原请求者投递的替代，不是另发一份；管理员发帖权限、负 ID 或频道用户名、失败不回退等规则见 [通知参考](notifications.md#频道与请求者投递)。

Cookie 失效时由部署者安全更新环境并重启，或删除该配置恢复 JSON 搜索。运行中的流失败不自动降级重搜；回退不重放中断搜索或未知写入。代理不读取真实 Cookie，不为验证运行真实资源搜索。详细认证边界见 [MoviePilot 参考](moviepilot.md#资源搜索流)。

Inline 需在 BotFather 开启 `/setinline`。
token 遗留的 allowed_updates 与 command scope 规则见 [Telegram 参考](telegram.md#启动命令与更新订阅)。
