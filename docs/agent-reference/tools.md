# 工具规程、指令来源与迁移记录

需要外部技术文档、解释终端输出，或核对本次指南拆分时读取。
本页不自动导入，也不替代仍然生效的运行器全局规则。

## 全局来源与管理边界

- Codex 全局入口为 `/root/.codex/AGENTS.md`，Claude 全局入口为 `/root/.claude/CLAUDE.md`；两者的 magpie 块由 Library 管理。
- 项目根保留精简沟通、Git 和数值硬限，便于项目迁移；这与当前全局规则的少量重复是方案 A 接受的取舍。
- Context7 的全局原文仍会注入。本页保存按需可查的规程，不宣称已经把该全局块移出上下文。
- Codex 终端规则在 `/root/.codex/RTK.md`，Claude 对应 `/root/.claude/RTK.md`。未加载时，在执行命令前手动读取当前运行器对应文件。
- 全局入口的相对 `@RTK.md` 指向各自目录内实际存在的文件；项目没有 RTK.md 不构成断链。
- Codex 全局入口还含指向同一 RTK 文件的绝对路径引用。这是文本层面的重复引用，不足以断言运行器会加载两遍。
- 不在本次项目重构中改写全局 magpie/RTK 文件或其管理源。真正调整全局注入须单独确定范围，并避免只改生成块后被同步覆盖。
- 项目 `CLAUDE.md` 保持原有一行 `@AGENTS.md`。目录指南和主题资料以手动路径读取，不追加详情导入。

## Context7 规程

当答案或实现依赖库、框架、SDK、API、CLI、云服务的具体行为时查询，包括语法、配置、安装、迁移和调试；即使熟悉也核验。
纯文案编辑、通用编程概念和完全本地逻辑跳过。重构、脚本和代码审查仅在外部 API 细节影响结论时需要查询。

1. 用官方名称解析库，问题写完整并带相关版本信息。

   ```sh
   npx ctx7@latest library "<library name>" "<specific question>"
   ```

2. 按名称、相关性、来源信誉、文档覆盖和 benchmark 选择匹配；必要时细化查询。
3. 获取所选库的文档。

   ```sh
   npx ctx7@latest docs <libraryId> "<specific question>"
   ```

4. 使用检索结果，引用支持判断的来源，尽量匹配安装版本；版本不匹配必须说明。

- 除非用户提供有效 library ID，否则先运行 `library`；不得捏造 ID 或版本后缀。
- 问题包含完整技术条件与相关版本，不包含凭据、私有代码或无关会话内容。
- 每个问题最多三条 Context7 命令，细化查询也计入。
- 配额错误必须报告，并说明 `npx ctx7@latest login` 或 `CONTEXT7_API_KEY` 可提供更高额度；不静默改为凭记忆回答。
- 文档不可得或不能得出结论时说明缺口；使用替代来源必须标明，优先官方资料。
- MoviePilot 行为不明时，除文档外核对对应版本源码。不能通过真实订阅、下载或 PT 搜索验证写路径。

## RTK 输出

当前 RTK 文件要求正常执行命令，把浓缩输出作为完整结果，并将相关命令合批。
截断结果会说明恢复路径；只有明显缺少应有输出、与退出码矛盾或内容乱码时，才用 `rtk proxy <cmd>` 重跑。
不要仅因为输出短就重复运行，也不要忽略输出明确给出的截断恢复提示。规则来源仍以对应运行器的受管理文件为准。

## 文档歧义的处理

本次只重组指南，不修改业务代码。下列范围说明来自已有实现和原文中的更具体规则；未验证的上游机制仍标记为待核实。

| 原表述 | 保存后的范围与依据 |
| --- | --- |
| 每条消息 rich、每条消息 Heading，同时 inline 为 classic HTML | 普通发送/编辑保持 rich 与 Heading；inline article 保留独立经典 HTML。`internal/telegram/inline.go` 与 `e2e/transcript_test.go` 已区分两者。 |
| 按钮只选择，URL 唯一例外为通知 | 选择按钮规则保留；通知 `flow.Open` 与 inline deep link 分属明确的链接出口，不把 inline 当作违反选择规则。 |
| 所有 entry 的 facts 在下一行 | 普通列表保留该布局；chart/related 的 `Brief` 同行是既有例外。 |
| 批量每个下载各一条通知，同时同媒体同季合并通知 | 保留每下载独立 watch；`internal/notify/state.go` 的 `due`、`holds`、`flush` 按同媒体同季聚合。没有新增每 watch 必须一条消息的要求。 |
| 任何按钮取消 follower，同时拒绝按钮恢复 follower | 先暂停并等待消息 lane；接受导航后旧身份失效，被拒 callback 恢复同一 follower 与 pending Reply，见 `internal/telegram/follow.go`。 |
| pending 为 chat/user，同时要求 topic 隔离 | 用完整 chat/user/topic 身份，见 `internal/telegram/address.go` 的 `inputOf`；二元描述只适用于无 topic 的简写。 |
| MoviePilot 拒绝都原文转达，同时敏感错误不展示 | 只有安全 `UserError` 原文转达；`call.secret` 响应与 execution error 原文不转发，防止回显凭据或路径。 |
| 取消订阅“停止下载” | 保留现有文案“不会再帮它搜索下载”，见 `internal/flow/subs.go`；不解释为自动删除或停止已存在 torrent。删除下载是另一个需确认的动作。 |
| 下载完成时关闭订阅，与种子加入时即计为取得 | 不据简写选定精确上游时点。保存共同不变量“订阅关闭不代表入库”，并保留原 v3.1.0 的取得集数记录；目标版本的触发条件仍须核对源码。 |
| 禁真实写入容易被读成禁 POST | 按副作用分类：只读媒体库 POST 与资源搜索 GET 不等价。前者仍限根允许的元数据查询，后者仍禁止。 |
| 全局禁止代码之后补单测，但也谈隔离测试 | 项目更严格地禁止全部单元测试与内部 mock。外部 HTTP fake 的 E2E 保留失败方式先枚举要求，不能把隔离要求当作单测许可。 |
| 函数最多 50 行，未说明空行 | 使用全局明确的非空行口径，不改变 50/3/3/10 的限制值。 |

README 的测试段仍有仅列两个 fake 的旧概述；实际已有 Bangumi fake。查实现入口时不能把该概述当作完整当前清单。

## 迁移账目

这是方案 A 的文档迁移记录，不是业务规范。原文基线为提交 `7a42a2f` 的 `AGENTS.md`（170 行、59,936 字节）；下列行号只指该基线。
保守拆分删除重复机制表达与仅复述签名的句子，保留实现索引、数值、上游理由、用户选择和录制来源；目录规则通过主题链接读取全部例外。

| 原位置 | 新位置与处理 |
| --- | --- |
| 1-3 | 根概况；平台计划移 [架构参考](architecture.md)，不作为实现承诺。 |
| 5-14、70-83 | 根常用检查与 [运行参考](operations.md)，完整命令、默认值、最小值、空值及零值语义保留。 |
| 16-28、34、38-68 的文件索引 | [架构参考](architecture.md) 保留入口；每条夹带的行为继续拆入下列主题，不以“索引”名义删除。 |
| 30、32、65-66 | [通知参考](notifications.md)：到货聚合、频道、画质、digest、原子状态与游标。 |
| 39-42、46、49-56、62-63 | [会话参考](flow.md)：意图、恢复、任务、首页、导航、订阅、历史、资源、多季、升级与分页。 |
| 43-45、47 | [发现参考](discovery.md)；相关推荐的导航与空结果行为在会话参考。 |
| 48、57、59-61、64 | [Telegram 参考](telegram.md)：inline、输入、渲染、海报、首页与消息 lane；原模型结构在会话参考。 |
| 58、89-96 | [会话界面规则](flow.md#界面与文案) 与语义模型；完整按钮顺序、标签、emoji、颜文字集合和安全语气保留。 |
| 87-88、97、103、170 | 根架构、数值硬限、E2E 与 Git 总则合并；不删除用户策略。 |
| 101 | 根常驻真实系统禁写与 PT 搜索禁令。 |
| 67-68、102-114 | [测试目录规则](../../e2e/AGENTS.md) 与 [E2E 参考](e2e.md)：verify、tap、协议、图像、时钟与录制来源。 |
| 118-121 | [MoviePilot 参考](moviepilot.md)：superuser、中文类型、响应 envelope、空订阅 null ID。 |
| 122 | Telegram 参考：rich、gallery、file_id、失效退化、Douban、预算、banner、旧图选择与旧照片迁移。 |
| 123-124 | MoviePilot 参考：w500、cosmetic details、director 混合字段不展示。 |
| 125 | 会话参考：start_episode、只追新集、同季条件与消费目标前的范围校验。 |
| 126-131 | Telegram 参考：命令 scope、other_bot、topic、allowed_updates、Follow、pending 重投及输入过期。 |
| 132 | MoviePilot 原始记录契约与通知下载 watch 范围，分别落在两个主题；不丢凭据限制、hash 关联与错误集数案例。 |
| 133-137 | 通知参考：按完成秒游标、迁移、grace、刷新范围、升级及可复用 row ID 的复合身份。 |
| 138-141 | 通知参考：quiet、库可见性、保存后发送；E2E 参考：full-poll barrier、Date 分页、同 ID retry 与图像识别 fake。 |
| 142 | 会话参考：64 字节上限、屏幕票据、过期指向和 finishing 的作用域。 |
| 143 | E2E 参考：SQLite row ID 复用、独立下载 hash 与覆盖方式。 |
| 144-147 | 会话参考：写前消费、终态回执、快捷选季、specials、随机 session、授权、复制 mutable slice 与订阅身份预检局限。 |
| 148-149 | MoviePilot 集数技术边界与会话动作唯一性；原有动作码索引仍保留。 |
| 150-151 | MoviePilot 每调用预算与私密错误；Telegram callback 锁、body deadline、cosmetic 并发、退出等待与共享连接池。 |
| 152-154 | 会话内容预算与省略、通知零等待及状态写、Telegram Notice 不编辑。 |
| 155-158 | MoviePilot 参考：库 unknown、去重、查询接口、下载字段、SSE 限制、任务身份、实例删除及删文件后果。 |
| 159-161 | MoviePilot 参考：类型过滤、暂停、删除已不存在、默认规则组、调度与执行时间/错误；合成敏感 fixture 来源另见 E2E。 |
| 162-163 | 发现参考：非 TMDB 身份、Bangumi 分组、并行简介、推荐与合集 workaround；fake 路由与并发归一化在 E2E。 |
| 164-165 | 通知参考：停滞与排队歧义、内存时钟/持久去重、Held 和实际下载到货分离、无法重建的旧观察。 |
| 166 | 根与 MoviePilot 参考：不确定上游行为须查源码。 |
| 全局 Communication/Git/Metrics/Context7/RTK | 原文件不动；根保留短摘要，本页说明来源、完整查询规程和工具管理边界。 |

核对不变量时应看新位置的正文与对应源码，不只看此表。拆分没有更新 golden、改变 API 行为或执行真实业务请求。
