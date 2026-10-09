# Application wiring

修改启动或装配前，读取 [架构入口](../../docs/agent-reference/architecture.md) 和 [运行与检查](../../docs/agent-reference/operations.md)。

- `internal/app` 是唯一具体依赖装配点，main 与 E2E 使用同一条 app 启动路径。
- `Backend`、`Now`、`*http.Client`、`*slog.Logger` 通过接口或依赖参数注入；不要在 flow 或适配器里另建生产依赖。
- 各客户端共享 transport；当前池设置与网络策略以 `transport.go` 为准，不以文档副本覆盖代码。
- 保留每调用超时与长轮询预算的区别；MoviePilot client 不能被统一 HTTP Timeout 截断资源搜索预算。
- Bot 停止轮询后仍需等待 handlers 与 followers；不能因 SDK 的 Run 返回就认为请求已经退出。
- flow 使用 app 注入的运行 context；handlers 退出后还要等待资源搜索与批量 API 提交 worker，不能让旧 worker 跨越一次 app 重启。
- 启动时验证所有声明的 banner 都有嵌入 JPEG，不能静默启动一个缺媒体资源的版本。
- 测试替换外部 HTTP 服务及可注入依赖，不建立与生产不同的业务装配路径。
