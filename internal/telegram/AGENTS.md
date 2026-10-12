# Telegram

修改接入、发送、渲染、海报或并发前，读取 [Telegram 详情](../../docs/agent-reference/telegram.md) 的对应小节。
修改文案、按钮或布局时，同时读取 [界面与文案规则](../../docs/agent-reference/flow.md#界面与文案)。

## 输入与身份

- 所有入口执行白名单检查；显式发给其他机器人的命令应在处理或拒绝前忽略。
- 请求、callback、typed answer 和通知都保留 chat/topic；仅 topic message 的正 thread ID 选择论坛话题。
- pending input 按聊天、用户、话题隔离；明确回复必须引用那个 prompt，群组不匹配回复保持安静。
- 私聊未引用文本回答最新有效 prompt，否则当标题搜索；识别的命令清除旧 prompt。
- 明确引用过期 prompt 只报告过期；未引用文本可直接新搜。通过语义字段判断，不检查渲染文案。
- 私聊文本和命令共用输入 lane，防止旧答案恢复已清除的 prompt。

## 发送与跟随

- 每条消息的状态变化和编辑串行，包括后台刷新；callback 在等待消息锁前暂停 follower。
- 被拒 callback 恢复同身份 follower 及未送达 Reply；接受导航后旧身份失效，不能恢复。
- 失败投递只重试同一 Reply，不重跑 flow transition 或资源搜索；遵守 `retry_after` 和重试截止。
- 普通编辑失败也保留终态回执；新消息发送结果未知时不重发。只有明确图片拒绝才降级海报或清除缓存。
- 初始发送迟到的响应不得恢复已被接受导航替代的 Follow 或输入提示；带请求者身份的通知发送前复核当前白名单。
- 发送、按钮编辑和 typed answer 编辑均按 `Reply.Follow` 启动跟随；`Reply.Notice` 不得编辑消息。
- `Reply.Wake` 只补充首批结果和终态的及时刷新；仍只读 Follow，限流与待投递 Reply 优先，不因关闭的唤醒通道忙循环或重跑业务。
- flow 决定结果后才回答 callback；提示随回答返回。需编辑的按下先预热该 Reply 的防盗链海报（最多 `warmBudget`，2s），再开始计 `answerLead`（默认 50ms）：编辑完成或运行满 `answerLead` 时回答，先到者为准，handler 返回前两者都已结束。用户可以在初始发送响应返回前点击已显示的按钮。
- `Reply.Prefetch` 的防盗链海报在发送/编辑成功后由后台下载（并发 2、可被停止取消、计入 Bot.Run 等待），不得延迟该 Reply；下载进程内去重，前台准备加入在途下载而非重复下载。
- 白名单用户的命令和私聊标题搜索（回复是新消息）运行满 `TypingLead`（默认 300ms，负值关闭）仍未返回时，向同一聊天和话题发 `sendChatAction typing`，每 4s 续一次；发送回复前停止并等待在途调用返回（不取消），不持有输入 lane 或消息锁。callback 与编辑 pending 提问的 typed answer 不发。
- 停止轮询后等待 handler 与 follower 退出，不假设 SDK 自动等待。

## 渲染与平台陷阱

- 普通发送/编辑使用 rich message，正文以 Heading 开头，所有用户和上游文本均转义。
- Inline article 是明确的经典 HTML 例外；保留其白名单、个人缓存、在途查询取消和 deep link 规则。
- 相同 token 可能保留旧命令 scope 和 `allowed_updates`；每次启动显式设置，不依赖 SDK 默认值。
- 只有首页使用 banner。不要为客户端编辑后暂留旧顶图的问题增加未经要求的 workaround。
- 海报 `file_id` 缓存、失效重发、无海报降级、Douban referer/UA、旧照片迁移按详情执行。
- 通知频道不能带请求者身份或失败回退到原聊天；源文件路径不能进入消息。
