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
| `e2e/run_mp_plugin_e2e.sh` | MP plugin E2E in the official MP image; `--update` rewrites its transcript |
| `crychic -version` | Print the build version and exit |
| `go test ./e2e/ -update` | Regenerate golden transcripts after an intended behavior change |

## 检查时机

Go 改动执行根指南的 build、vet、gofmt 检查与 race E2E；容器改动再核对 Docker 构建。`go vet` 和 `gofmt -l` 均须无输出，退出码与输出一起判断。

有意改变交互行为才用 `go test ./e2e/ -update`，之后逐行审阅 transcript。普通测试不会重写 golden。失败或不能执行的检查如实报告，不能为提交绕过。

## 版本与 CI

机器人版本来自 git tag（`vX.Y.Z`）。仓库内 `go build` 由 Go 工具链写入 tag 或伪版本，有未提交改动时带 `+dirty`；Docker 上下文不含 `.git`，须用 `--build-arg VERSION="$(git describe --tags --always --dirty)"` 注入，compose 示例读取 `CRYCHIC_VERSION`，未注入时为 `dev`。启动日志和 `-version` 输出该版本，镜像标签 `org.opencontainers.image.version` 同值。

发版规则（主人 2026-10-11 授权，代理直接执行，不逐次确认）：

- 只在构建部署镜像时打 tag，不按提交打；部署镜像用该 tag 构建，使线上版本可对应到提交。
- 0.x 阶段，上一 tag 以来含 `feat` 或不兼容改动升 minor，仅有 `fix`、文档等改动升 patch；只有文档、CI 等不进二进制的改动不打 tag。
- 在已提交且检查通过的 HEAD 上打附注 tag（`git tag -a vX.Y.Z -m "Crychic vX.Y.Z"`），交付时报告版本与依据。
- 只打本地 tag；push 分支或 tag 仍需主人明确要求。
- 升 1.0.0、跳号、无法判断是否不兼容时先问主人。

MP 插件版本独立维护：`package.v3.json` 的 `version`、`history` 与 `__init__.py` 的 `plugin_version` 必须一致，只在插件改动时提升，避免宿主提示无意义升级。

`.github/workflows/ci.yml` 在 push main、`v*` tag 与 PR 时运行：Go 构建、vet、gofmt、race E2E；插件版本一致性及隔离宿主 E2E；Docker 构建与 `-version` 检查。CI 只用 fake 与隔离镜像，不发布镜像，不接触真实服务。

## 环境变量

- `CRYCHIC_MOVIEPILOT_URL`, `CRYCHIC_MOVIEPILOT_API_KEY` (MoviePilot `API_TOKEN`), `CRYCHIC_TELEGRAM_TOKEN` - required
- `CRYCHIC_MOVIEPILOT_COOKIE` - optional resource Cookie value; enables non-Agent resource-search SSE, empty keeps JSON search. No Cookie header, automatic login or renewal. Public DNS names require HTTPS; private/loopback IPs and single-label service names may use HTTP. Never print or persist the value.
- `CRYCHIC_TELEGRAM_ALLOWED_USERS` - required, comma-separated user IDs; empty refuses to start
- `CRYCHIC_TELEGRAM_API_URL` - optional, defaults to `https://api.telegram.org`
- `CRYCHIC_BANGUMI_API_URL` - optional, defaults to `https://api.bgm.tv`; the 新番放送 calendar is read from it
- `CRYCHIC_DATA_DIR` - optional, defaults to `data`; holds `requests.json` and `native-notifications.json`
- `CRYCHIC_NOTIFY_INTERVAL` - optional, defaults to `1m`, minimum `100ms`; request-fact polling and failed native-reception retry interval
- `CRYCHIC_PROGRESS_INTERVAL` - optional, defaults to `5s`, minimum `100ms`; task and resource-list refresh cadence. SSE first results and terminal state wake the existing follower early; intermediate frames do not each trigger an edit.

## 配置与部署入口

配置及最小间隔见 [config.go](../../internal/config/config.go)；公开用法见 [README](../../README.md#配置)，容器定义见
[Dockerfile](../../Dockerfile) 和 [compose 示例](../../compose.example.yaml)。

镜像使用 distroless、uid 65532，持久数据挂载 `/data`。原项目部署示例采用 host 网络；只有首页 banner 随二进制嵌入，不能误把缺媒体资源当可忽略配置。

通知使用 MP 自定义 crychic 渠道，默认目标与话题在插件页面设置，用户范围和发送时段继续由 MP 决定。旧通知业务变量及频道变量仅提示名称后忽略，不回显值。见 [通知参考](notifications.md)。

Cookie 失效时由部署者安全更新环境并重启，或删除该配置恢复 JSON 搜索。运行中的流失败不自动降级重搜；回退不重放中断搜索或未知写入。代理不读取真实 Cookie，不为验证运行真实资源搜索。详细认证边界见 [MoviePilot 参考](moviepilot.md#资源搜索流)。

Inline 需在 BotFather 开启 `/setinline`。
token 遗留的 allowed_updates 与 command scope 规则见 [Telegram 参考](telegram.md#启动命令与更新订阅)。
