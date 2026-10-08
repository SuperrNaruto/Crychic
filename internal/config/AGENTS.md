# Configuration

修改环境变量或默认值前，读取 [运行与配置](../../docs/agent-reference/operations.md) 及 `config.go`。

- URL、MoviePilot API key、Telegram token 与非空用户白名单必填；空白名单拒绝启动，不得默认为全部允许。
- 保留通过传入 `getenv` 读取配置的边界，避免在配置解析内部直接依赖进程环境。
- 通知目的地只接受负聊天 ID 或 `@channelusername`；空值表示请求聊天，不能悄悄改变隐私范围。
- 轮询与刷新间隔的最小值、quiet/library wait/stall 的零值关闭语义、digest 空值关闭语义分别保留。
- digest 的星期与时刻以中国时间解释；配置值和错误不得带出真实密钥或 token。
- 变更公开配置时同步检查根 README、compose 示例与运行参考；源码是当前默认值与校验的事实来源。
