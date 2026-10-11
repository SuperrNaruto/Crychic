# Configuration

修改环境变量或默认值前，读取 [运行与配置](../../docs/agent-reference/operations.md) 及 `config.go`。

- URL、MoviePilot API key、Telegram token 与非空用户白名单必填；空白名单拒绝启动，不得默认为全部允许。
- 保留通过传入 `getenv` 读取配置的边界，避免在配置解析内部直接依赖进程环境。
- 原生通知的目标在 MP 自定义渠道及插件页面配置；旧频道、quiet、library wait、stall、digest 变量只提示名称后忽略，不解析或回显值。
- Notify interval 只控制原生通知接入失败后的重试，不影响 MP 发送时段；正常通知使用长轮询。
- 变更公开配置时同步检查根 README、compose 示例与运行参考；源码是当前默认值与校验的事实来源。
