# Crychic 原生通知渠道

实现 MP 的自定义通知渠道，并以 API key 提供整理进度只读投影；现有 Go 机器人仍是唯一 Telegram 接入端。默认不开启；不创建 MP 用户、渠道或机器人，不修改既有渠道和用户绑定。

## 安装与配置

1. 使用仓库 `package.v3.json` 和 `plugins.v3/crychic/` 作为 MP v3 插件包。未推送的本地提交不会出现在远程插件市场；安装、复制源码及重载由主人执行。
2. 在 MP 通知设置中新增「自定义」，类型填 `crychic`，名称自行指定，选择通知类型并启用。
3. 打开 Crychic 插件配置页，按三步引导填写。在「通知渠道」中按名称选择已有 crychic 渠道；只有一条时可以保留自动选择，多条时明确选择对应名称。开启接入后保存。
4. 默认目标支持正用户 ID、负群组或频道 ID 及 `@channelusername`。用户范围由 MP 决定；用户个人 Telegram 绑定优先，未绑定用户不回退默认管理员。话题仅适用于负数目标。
5. Go 应用继续使用原有 MP 地址、API key 和 Crychic Bot token。MP 内不填写该 Bot token，不启动原生 Telegram 客户端。

管理员 API key 只认证接入，不代表聊天用户。个人接收仍遵守 Go 应用的白名单，频道投递不增加请求者身份。默认目标不会被旧 `CRYCHIC_TELEGRAM_NOTIFY_CHAT_ID` 覆盖。

配置页及详情页共用创建渠道、接入参数、机器人配置与新通知核对的引导。字段说明常驻显示，桌面两列、窄屏单列；读取页面保留已保存参数，只读取渠道元数据。目录为空或读取失败时显示对应提示，并保留原选择。API key、Bot token 和白名单仍在 Go 端配置，页面只展示配置项名称。

## 接口与状态

默认由 MP 的 API-key 依赖保护，无匿名入口。四个接口均使用 `{success,data}`，失败只返回固定安全代码。

- `GET /api/v1/plugin/Crychic/status` 返回协议版本、渠道、稳定 stream 和 head。
- `GET /api/v1/plugin/Crychic/notifications` 接受 `after`、`consumer_id`、`channel_id`、`stream_id` 和 `wait`；最长等待 25 秒，每批最多 32 条，且编码后不超过 1 MiB（首条总返回，按顺序取连续前缀），读取不删除记录。图片与链接超过 4096 字符时不转发。
- `POST /api/v1/plugin/Crychic/ack` 接受同一来源与消费身份、`sequence`；幂等确认并清理安全投影，不宣称 Telegram 发送成功。
- `GET /api/v1/plugin/Crychic/transfer-progress`（v1.2.0）返回整理队列中运行文件的进度，`data.files` 每项为 `media_source`、`media_id`、`season`、`begin_episode`、`end_episode`、`download_hash`、`downloader`、`active` 与 `percent`。只要求插件已启用，与通知渠道配置无关；失败代码为 `progress_unavailable`。

原生 `/api/v1/system/progress/{key}` 只接受 30 分钟有效的资源 Cookie，`RESOURCE_SECRET_KEY` 未固定时随 MP 重启轮换，Go 无法用 API key 稳定读取；公开 SDK 也没有整理队列入口。因此该接口只读 `/transfer/queue` 的同一来源（单例 `TransferChain` 的作业视图）及宿主进度缓存，键沿用写入方 `transfer_process` 的 `md5(Path(fileitem.path).as_posix())`，与官方整理队列页面一致，最多 12 个文件。不修改队列或进度，不返回路径、文件名、进度文本或错误；没有进度记录的运行文件（如硬链接）不出现。复制失败中断时宿主可能留下未结束的旧记录，Go 只在文件仍运行且身份唯一时展示。

协议版本为 1，通知包含独立 ID、sequence、platform、recipient、title、text、image、link。首版平台为 Telegram；不在插件中生成 HTML，也不根据正文计算媒体业务结论。平台适配与未来 Discord 接入留在 Go 端。

插件自有数据库保存未消费安全投影、稳定顺序、排他租约及最小去重身份；不保存原始 Message、未知附件、密钥或敏感路径。租约为 60 秒，重启使用同一消费身份恢复。来源或游标异常停止接入，不自动清空队列或改用历史/SSE。

v1.0.1 在图片和链接入队前检查已知凭据、百分号编码以及敏感参数，空敏感参数也会被过滤。读取旧队列时同步修正不安全 URL，保留消息身份、顺序、标题和正文；畸形 URL 只移除对应图片或链接。安全 URL 保持原样。

长轮询在通知事务提交后由入队事件唤醒，每 5 秒复核数据库，并独立检查断线。等待期限使用单调时钟，包含读取所耗时间；已开始的数据库调用仍需等待结束。消费租约剩余不超过 30 秒时才续期，避免空闲等待持续写库。重载会结束旧代 HTTP 等待，持久来源和游标继续保留，无新增必需配置。

Go 在 `native-notifications.json` 中先原子记录投递尝试，再发送和确认；结果未知不重放。这个取舍仍可能漏发，不承诺严格一次且仅一次送达。首次基线跳过接入前记录，普通重启不重新建立基线。

## 宿主兼容点

本次以 MP 前后端 v3.1.4 核验。整理进度另以内部适配读取 `TransferChain().get_queue_tasks()` 与 `ProgressHelper`，宿主 E2E 用真实作业视图和 `transfer_process` 写入方核对键规则与投影。公开 SDK 用于插件声明、API 和自有数据库；窄范围适配复用 `_MessageBase` 的渠道检查与目标辅助方法，并包装 `NotificationMixin.post_message_strict` 的可信事件身份。普通请求关联 ID 不能当消息去重键。停止时仅撤销自己安装的包装，其他渠道继续执行。

宿主调度先执行插件模块、再执行系统渠道；durable 通知走 strict 调度，插件异常会中断本轮系统渠道并触发 outbox 重试。因此捕获阶段的任何失败，包括渠道缺失或歧义、目标无效、正文过长及插件数据库失败，都只记录固定代码并放弃本渠道这一条，不抛回宿主。代价是插件数据库的瞬时故障会漏掉这条 Crychic 通知，不靠 MP 重试补回。

## 验证与回退

真实 Go app 对外部 HTTP fake 的场景在 `e2e/`。真实 MP 宿主测试使用官方镜像 `jxxghp/moviepilot-v3:3.1.4`，不连接真实服务，不加载生产配置；执行命令见 `e2e/run_mp_plugin_e2e.sh`。产物为 `e2e/testdata/transcripts/MPPluginNativeHost.txt`。

宿主验收另覆盖整理进度只投影有记录的运行文件、排除等待中、无记录和全局进度且不含路径，以及整理失败通知路径在入队和旧队列读取时隐藏、最长正文按编码字节分批、安全 URL 配对、编码凭据、旧队列修正、空闲数据库预算、入队唤醒、消费者争抢与重载等待隔离。脚本输出合成三秒空闲等待的真实进度读取与租约写入数量，transcript 保存预算和行为结论，不记录通知原文、凭据或不稳定耗时。

停用插件只停止通知接入和整理百分比，Go 其他功能继续运行，持久队列保留。旧请求状态首次迁移前保留 `requests.json.pre-native`；回退需先停 Go 并由主人选择备份状态，不能自动恢复旧周报、pending 或未知投递。不要让旧版本地通知引擎和新渠道同时投递。
