# End-to-end tests

新增、修改场景、fake、fixture 或 golden 前，读取 [E2E 详情](../docs/agent-reference/e2e.md) 的对应小节。
根规则中的真实 MoviePilot 禁写与 PT 搜索禁令同样适用于录制和排障。

## 行为与产物

- 真实 app 对外部协议 fake，不 mock 内部包，也不为测试另建业务路径。
- 缺陷修复只有真实行为覆盖缺口才加场景；不添加冗余或仅检测实现变化的测试。
- 每个场景以 `h.tr.verify(t)` 结束。普通运行比对已提交 transcript；`-update` 才写 golden 文件。
- 只有有意行为变化才更新 golden，逐行审阅差异，并与对应代码同提交。
- 正常按钮用 `tap(user, msgID, label)`，缺按钮就失败；`tapData` 只用于故意陈旧的按钮。
- 普通发送/编辑正文忽略顶端海报后必须以 `<h3>` 开头，包括失败、过期与结束消息；inline article 使用其独立格式检查。
- 仅在记录产物时归一化不透明 session ID，不改送进 app 的更新，保留跨重启身份验证。
- 并发读取只归一化同一无序读取组的顺序，不掩盖有序交互或写操作。

## 外部服务与数据

- fake 保持真实长轮询、认证、响应包裹、错误和有副作用的请求语义；扩展 fake，不走捷径。
- 订阅 row ID 可以复用，下载删除必须按实例及 hash；不能用单调计数或默认实例掩盖身份问题。
- fake 必须支持消息显示早于发送响应返回，不能让 callback 依赖发送者已学到 poster file_id。
- transcript 比较实际显示图像的稳定身份，不比较 URL/upload/file_id 的传输形式；缓存场景单独验证 file_id 复用。
- 时钟偏移不改变真实 timer cadence；轮询屏障等完整处理一轮，不能以碰巧 sleep 足够作为条件。
- 录制只留 Crychic 读取的字段；真实密钥、cookie、passkey、用户信息和私有服务器地址不得进入仓库或聊天。
- 写响应使用符合目标版本源码的合成 fixture；已有一次性真实订阅录制不能再次制造真实订阅来更新。
- 保留 fixture 来源、合成例外及敏感错误验证用例；具体协议、图像、时钟、分页与路由规则见详情。
