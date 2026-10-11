"""MP 原生自定义 crychic 通知渠道、持久长轮询接入与整理进度只读投影。"""

import asyncio
import inspect
from fastapi import Request
from app.sdk.config import settings
from app.sdk.logging import logger
from app.sdk.plugin import _PluginBase
from app.sdk.services import NotificationHelper
from .adapter import EVENT_KEY, EventIdentity, NotificationMixin, Policy
from .configuration import CHANNEL_TYPE, configuration_form, setup_steps
from .models import Pending, Progress, Receipt
from .progress import running_progress
from .projection import project, recipient
from .store import Outbox

MAX_WAIT = 25
DISCONNECT_INTERVAL = 1
RECHECK_INTERVAL = 5


class Crychic(_PluginBase):
    """实现通知渠道与整理进度只读投影，不接收 Telegram 用户更新或重写业务通知。"""

    plugin_name = "Crychic"
    plugin_desc = "MP 自定义通知渠道，持久长轮询接入现有 Crychic 机器人，并提供整理进度只读投影。"
    plugin_version = "1.2.0"
    plugin_author = "Crychic"
    plugin_config_prefix = "crychic_"
    plugin_order = 100
    auth_level = 1

    def init_plugin(self, config=None):
        """读插件参数，不自动创建或改写 MP 渠道与用户绑定。"""
        self.stop_service()
        self.config = dict(config or {})
        self.enabled = bool(self.config.get("enabled", False))
        previous = getattr(self, "outbox", None)
        self.outbox = Outbox(self.get_database, secrets=(getattr(settings, "API_TOKEN", ""),),
                             lock=previous.lock if previous else None)
        self.identity = EventIdentity()
        self.compatible = self.check_compatibility()
        if self.enabled and self.compatible:
            self.identity.install()

    @staticmethod
    def check_compatibility():
        """仅接受已核对的必要宿主契约，未知结构不绕过。"""
        methods = ("check_message", "get_notification_targets", "get_notification_userid")
        if not all(callable(getattr(Policy, name, None)) for name in methods):
            return False
        return "event_key" in inspect.signature(NotificationMixin.post_message_strict).parameters

    def get_state(self):
        """只有接入参数与宿主契约有效时发布模块扩展。"""
        return bool(getattr(self, "enabled", False) and getattr(self, "compatible", False))

    def get_module(self):
        """原生调度之后接入；捕获函数始终不短路其他渠道。"""
        return {"post_message": self.capture_notice}

    def channel(self):
        """匹配稳定渠道 ID；只有单条自定义渠道时允许省略选择。"""
        items = [c for c in NotificationHelper().get_configs().values() if c.type == CHANNEL_TYPE]
        selected = self.config.get("channel_id")
        if selected:
            items = [c for c in items if c.id == selected]
        if len(items) != 1:
            raise ValueError("channel_missing_or_ambiguous")
        return items[0]

    def capture_notice(self, message, **kwargs):
        """插件模块先于系统渠道执行；strict 调度会传播异常，故任何失败只停本渠道。"""
        try:
            self.enqueue(message)
        except Exception as err:
            fixed = type(err) is ValueError and err.args and str(err.args[0]).isidentifier()
            code = err.args[0] if fixed else type(err).__name__
            logger.warning("Crychic 通知未接入：%s", code)
        return None

    def enqueue(self, message):
        """复用原生检查、已解析受众及持久缓冲，绝不返回全局接管结果。"""
        if not self.get_state():
            return
        channel = self.channel()
        default = self.config.get("default_target", "")
        policy = Policy(channel, default)
        if not policy.check_message(message, channel.name):
            return
        target = recipient(policy, message, default_target=default, topic=int(self.config.get("topic_id", 0)))
        payload = project(message, target, channel=channel.id, event_key=EVENT_KEY.get(),
                          secrets=(getattr(settings, "API_TOKEN", ""),))
        self.outbox.append(channel.id, payload)

    def get_database_models(self):
        """声明插件自有模型，宿主在初始化后建立表。"""
        return [Progress, Pending, Receipt]

    def get_api(self):
        """默认 API-key 认证；无匿名入口，不返回原始错误。"""
        return [dict(path="/status", endpoint=self.status, methods=["GET"], auth="apikey"),
                dict(path="/notifications", endpoint=self.notifications, methods=["GET"], auth="apikey"),
                dict(path="/ack", endpoint=self.ack, methods=["POST"], auth="apikey"),
                dict(path="/transfer-progress", endpoint=self.transfer_progress, methods=["GET"], auth="apikey")]

    def status(self):
        """返回协议状态和必要进度，不带消息内容。"""
        try:
            if not self.get_state():
                raise ValueError("inactive_or_incompatible")
            return dict(success=True, data=self.outbox.status(self.channel().id))
        except Exception:
            return dict(success=False, code="native_unavailable", data=None)

    async def notifications(self, request: Request, *, after: int, consumer_id: str,
                            channel_id: str, stream_id: str, wait: int = MAX_WAIT, limit: int = 32):
        """长轮询不删除正文，退出与断线及时结束等待。"""
        query = dict(after=after, consumer_id=consumer_id, channel_id=channel_id, stream_id=stream_id)
        try:
            return await self.wait_notifications(request, query, wait=wait)
        except Exception:
            return dict(success=False, code="native_unavailable", data=None)

    async def wait_notifications(self, request, query, *, wait):
        """入队唤醒与低频核对分开，真实期限包含数据库等待。"""
        loop = asyncio.get_running_loop()
        outbox = self.outbox
        deadline = loop.time() + min(max(wait, 0), MAX_WAIT)
        next_read, changed = 0, True
        while self.get_state() and self.outbox is outbox:
            if await request.is_disconnected():
                return dict(success=False, code="disconnected", data=None)
            if changed or loop.time() >= next_read:
                version = outbox.version()
                data = await asyncio.to_thread(self.read_notifications, outbox, query)
                next_read = loop.time() + RECHECK_INTERVAL
                if not self.get_state() or self.outbox is not outbox:
                    break
            remaining = deadline - loop.time()
            if data["messages"] or remaining <= 0:
                return dict(success=True, data=data)
            delay = min(DISCONNECT_INTERVAL, next_read - loop.time(), remaining)
            changed = await asyncio.to_thread(outbox.wait, version, max(delay, 0))
        return dict(success=False, code="native_unavailable", data=None)

    def read_notifications(self, outbox, query):
        """在线程内核验当前渠道并打开独立数据库会话。"""
        return outbox.read(self.channel().id, query)

    async def ack(self, request: Request):
        """只确认消费进度，不声明外部投递或业务操作成功。"""
        try:
            if not self.get_state():
                raise ValueError("inactive")
            query = await request.json()
            return dict(success=True, data=await asyncio.to_thread(self.outbox.ack, self.channel().id, query))
        except Exception:
            return dict(success=False, code="ack_rejected", data=None)

    async def transfer_progress(self):
        """原生进度只认资源 Cookie，这里以 API key 只读投影运行中文件；与通知渠道配置无关。"""
        try:
            if not getattr(self, "enabled", False):
                raise ValueError("inactive")
            files = await asyncio.to_thread(running_progress)
            return dict(success=True, data=dict(files=files))
        except Exception:
            return dict(success=False, code="progress_unavailable", data=None)

    def get_form(self):
        """渠道类型开关和业务受众仍由 MP 原生页面配置。"""
        return configuration_form()

    def get_page(self):
        """展示不含凭据的配置说明。"""
        return setup_steps()

    @staticmethod
    def get_command():
        """不接管任何机器人命令。"""
        return []

    def stop_service(self):
        """停接入与等待，保留持久队列，不关闭宿主数据库引擎。"""
        self.enabled = False
        outbox = getattr(self, "outbox", None)
        if outbox:
            outbox.close()
        identity = getattr(self, "identity", None)
        if identity:
            identity.stop()
