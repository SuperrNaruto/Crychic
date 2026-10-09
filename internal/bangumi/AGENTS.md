# Bangumi

修改日历、简介或跨来源身份时，读取 [发现详情](../../docs/agent-reference/discovery.md)。

- 日历直接读取 Bangumi 公共 API，保留全部星期分组，不换成丢分组的 MoviePilot recommend 日历。
- 日历简介优先 TMDB、MoviePilot Bangumi API，再在剩余整页补充预算内回退公共 subject API；上游空结果不能伪装成成功注释。
- Bangumi 身份与 TMDB 转移身份不同，保留 `CalendarID`，不得仅用同名推定同一媒体。
- TMDB 匹配保留 pick 的名字、星期与首播日期；续季的年份例外需要维持详情里的匹配条件。
- 星期和播出日期以中国时间的 civil date 语义处理，不从 UTC 时间戳猜一天。
- HTTP client 与 API URL 由 app 注入；读取失败按既有降级规则处理，不扩大服务写入范围。
- 变更录制或 fake 时先读 [E2E 规则](../../e2e/AGENTS.md) 和 [录制来源](../../docs/agent-reference/e2e.md)。
