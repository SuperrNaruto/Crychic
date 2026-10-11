# Crychic

Go 聊天机器人；通过 MoviePilot v3 搜索媒体、订阅或下载选中的资源，使用用户白名单鉴权。

## 协作与适用范围

- 默认用中文，先给结果；影响结果的取舍、限制与未完成验证必须说明。
- 直接完成任务；不用恭维、空泛安慰或填充句，不在结尾主动追加任务或邀约。
- 本文件只保留跨目录规则；修改或审查某目录前，显式读取下表中的对应 AGENTS.md。
- 不假设客户端会自动加载嵌套 AGENTS.md；跨目录任务读取所有相关目录规则。
- 详情文件按表中触发条件手动读取，不使用 `@` 导入。
- 根 CLAUDE.md 保持唯一一行 `@AGENTS.md`；不要复制维护第二套项目规则。
- 局部规则保留必须遵守的行为边界；详情保存实现说明、设计原因、平台陷阱及既有测试证据。
- 旧实现说明不等于当前验证结果；发现与代码不符时说明差异，不擅自丢弃其中的安全约束或用户选择。

## 后续开发原则

- 采用「现有 Crychic + 原生 API 优先 + MP 插件补足能力」，保留现有 Go 机器人，不将整个项目重写成 MP 插件。
- 新功能先核验 MP 原生 API；能够可靠满足需求时直接使用，不为统一入口让所有请求经过插件。
- 仅在 API 存在明确的能力、事件、上下文或可靠性缺口时增加 MP 侧 Crychic 插件扩展，记录缺口与采用理由。
- 插件保持薄适配层，优先使用公开 SDK 和受支持的扩展点，复用 MP 原生业务实现；现有鉴权、隐私、确认及真实服务操作边界不变。
- 分工与选择顺序见 [架构参考](docs/agent-reference/architecture.md#后续开发原则)；扩展方向不等于已经实现或验证通过。

## 安全边界

- 代理绝不写真实 MoviePilot，也不运行最终会创建订阅或下载的真实机器人流程。
- 禁止真实订阅写入、暂停/恢复/删除，以及下载创建/删除；示例包括 `POST /api/v1/subscribe/` 和 `POST`/`DELETE /api/v1/download/`。
- 禁止调用真实 `/api/v1/search/*`：这是会访问主人的 PT 站点的资源搜索，并非普通元数据查询。
- 真实服务仅允许只读元数据查询；写路径用端到端 fake 与 MoviePilot 源码验证，实机写入由主人手测。
- MoviePilot API key 是超级用户身份，不代表聊天用户；不能据此推断请求者有操作权限。
- 当前按单一信任域运行：白名单内用户同等管理 MP 内全部订阅与下载，不维护本地请求归属。多用户隔离后置到 MP 官方支持按用户的 API 凭据，届时按 `Actor` 选择凭据并沿用 MP 自身的用户归属，不重建本地归属表；理由见[架构参考](docs/agent-reference/architecture.md#用户与权限模型)。
- 消息、日志、录制与提交不得泄露真实密钥、token、cookie、passkey、服务器路径或敏感响应。
- 含站点凭据的下载原始记录不展示、不记日志、不持久化；仅在内存中保留并原样交回 MoviePilot。
- 录制材料只保留所需字段并脱敏；错误、调试输出和测试产物也遵守这些隐私边界。

## 跨目录代码约束

- `flow` 不导入平台包；新平台与 `internal/telegram` 平级，经小接口消费 flow。
- 注入 Backend、Now、HTTP client 和 logger；仅在 `internal/app` 构造与装配具体依赖。
- 函数最多 50 个非空行，嵌套最多 3 层，最多 3 个位置参数，圈复杂度最多 10。
- 超限立即抽取辅助函数或选项结构；使用具名常量，不写魔法数字。

## 文档核验与工具边界

- 依赖外部库、框架、SDK、API、CLI 或云服务具体行为时，遵守全局 Context7 规则；来源见 `docs/agent-reference/tools.md`。
- 纯文字编辑、通用编程概念和完全基于本地逻辑的工作跳过查询；重构、脚本或审查仅在外部 API 细节影响结论时查询。
- MoviePilot 行为不明确时查其源码，不凭记忆补出接口行为；源码入口与版本记录见 MoviePilot 详情。
- 后续新增能力默认核对 MP 最新稳定版，保留已有历史核验记录，不例行重复核对多个旧版本；目标部署存在明确版本差异时再针对差异验证。
- 若未加载终端输出规则，执行命令前手动读取当前运行器的 `/root/.codex/RTK.md` 或 `/root/.claude/RTK.md`。
- 全局 magpie 和 RTK 文件保持其管理边界；项目重构不直接改写这些文件，也不宣称按需详情替代了全局注入。

## 验证与提交

- 测试仅用端到端方式：真实 app 对外部服务 HTTP fake；不写单元测试，不 mock 内部包。
- 不增加冗余测试、变更侦测测试，或没有真实行为覆盖缺口的修复回归测试。
- 若确需隔离外部系统，先列出全部可能失败方式，再实现；保持端到端边界，不改用内部单测。
- 行为验证须有可重复核验的产物；场景、transcript 比对与有意更新的规则见 `e2e/AGENTS.md`。
- 完成任务后执行适用检查；失败或不可用必须报告，不跳过检查强行提交，也不声称未验证内容已通过。
- 只暂存本任务改动，验证后及时单独提交；保留他人已暂存与未暂存工作。
- 未经明确要求不 push、amend、重写历史或丢弃现有工作；只读提案不改文件、不创建提交。
- 交付时报告 commit ID 与 push 状态；未提交时说明原因。
- 部署时按 operations 详情「版本与 CI」的发版规则自行决定 SemVer 并打本地附注 tag，无需逐次确认；升 1.0.0、跳号或不确定兼容性时先问。

## 常用检查

- 构建：`go build ./cmd/crychic`。
- 静态检查与格式检查：`go vet ./... && gofmt -l .`，两者都应无输出。
- 完整端到端检查：`go test -race ./...`；测试仅放在 `e2e/`。
- MP 插件宿主 E2E：`e2e/run_mp_plugin_e2e.sh`；CI 在 push、tag 与 PR 时运行上述全部检查。
- 单场景、golden 更新、Docker 构建、版本号及部署参数见 `docs/agent-reference/operations.md`。

## 局部规则导航

| 涉及范围 | 修改或审查前读取 | 保存的规则 |
| --- | --- | --- |
| `internal/flow/` | `internal/flow/AGENTS.md` | 身份与确认票据、写入一次与未知结果、导航、语义文案与布局、列表边界 |
| `internal/moviepilot/` | `internal/moviepilot/AGENTS.md` | 响应解包、身份与枚举、安全错误、调用预算、下载实例、集数解析 |
| `internal/telegram/` | `internal/telegram/AGENTS.md` | 白名单、输入与话题、callback 时序、follower、富文本、海报与隐私 |
| `internal/notify/` | `internal/notify/AGENTS.md` | 通知文件名边界、先保存后发送、游标与迁移、watch 身份、到货合并与完成 |
| `internal/bangumi/` | `internal/bangumi/AGENTS.md` | 日历与简介接口边界、身份字段和降级 |
| `internal/app/` | `internal/app/AGENTS.md` | 唯一装配点、共享传输与退出等待 |
| `internal/config/` | `internal/config/AGENTS.md` | 环境配置、白名单与默认值语义 |
| `plugins.v3/crychic/` | `plugins.v3/crychic/AGENTS.md` | 原生自定义渠道、宿主适配、持久安全投影与真实宿主验收 |
| `e2e/` | `e2e/AGENTS.md` | 行为缺口、真实 fake、交互入口、脱敏录制、并发与时钟、transcript 比对及更新 |

## 按需详情导航

以下文件保存原文中仍有用的详细说明；不是每次任务都要读的指令。

| 触发条件 | 手动读取 |
| --- | --- |
| 首次跨模块定位、修改数据流或寻找已有实现 | `docs/agent-reference/architecture.md`：目录职责、数据流与关键文件索引 |
| 改会话、搜索、导航、订阅、历史、下载，或任意目录的用户可见文案 | `docs/agent-reference/flow.md`：行为、动作码、限制、用户偏好与设计原因 |
| 改 MoviePilot 请求或解析、接口 fake，或行为不明 | `docs/agent-reference/moviepilot.md`：v3.1.0 观察、接口契约、陷阱、源码入口 |
| 改 Telegram 接入、消息、图片、并发或输入 | `docs/agent-reference/telegram.md`：富文本与 inline 例外、缓存、预算、平台陷阱 |
| 改通知、持久化、迁移或到货语义 | `docs/agent-reference/notifications.md`：状态与身份、合并、游标、等待、升级、停滞、摘要 |
| 改 Bangumi 或 TMDB 日历、图表身份匹配 | `docs/agent-reference/discovery.md`：来源、续季身份、并行查询、简介与系列选择 |
| 改测试、fixture、golden 或调试时间与顺序问题 | `docs/agent-reference/e2e.md`：harness、fake 契约、录制来源、确定性与既有用例 |
| 改配置、启动、部署、运维或执行专项检查 | `docs/agent-reference/operations.md`：完整命令、环境变量、默认值、Docker 与启动约束 |
| 需要外部技术文档或处理工具输出问题 | `docs/agent-reference/tools.md`：全局 Context7 来源、查询规程入口与 RTK 管理边界 |
