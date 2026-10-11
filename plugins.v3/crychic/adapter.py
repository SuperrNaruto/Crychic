"""窄范围宿主适配，复用路由检查，不创建任何平台客户端。"""

import copy
from contextvars import ContextVar
from functools import wraps
from app.modules import _MessageBase
from app.chain._messaging import NotificationMixin

EVENT_KEY = ContextVar("crychic_notice_event_key", default=None)


class Policy(_MessageBase):
    """自定义渠道的原生检查器，不物化 Telegram 服务。"""

    _notification_user_target_keys = ("telegram_userid",)
    _notification_admin_default_config_keys = ("DEFAULT_TARGET",)

    def __init__(self, config, default_target):
        """只提供当前配置给原生检查，显式他平台消息不被接管。"""
        super().__init__()
        config = copy.deepcopy(config)
        config.config = dict(config.config or {}, DEFAULT_TARGET=default_target)
        self._configs = {config.name: config}


class EventIdentity:
    """仅标记原生 durable 事件身份，不把普通请求 ID 当作消息 ID。"""

    def __init__(self):
        """保留原入口，停用时只撤销自己安装的包装。"""
        self.original = None
        self.wrapper = None

    def install(self):
        """原函数照常执行；额外上下文只用于消息去重。"""
        if self.wrapper is not None:
            return
        original = NotificationMixin.post_message_strict

        @wraps(original)
        def wrapped(host, message, *, event_key):
            token = EVENT_KEY.set(event_key)
            try:
                return original(host, message, event_key=event_key)
            finally:
                EVENT_KEY.reset(token)

        self.original, self.wrapper = original, wrapped
        NotificationMixin.post_message_strict = wrapped

    def stop(self):
        """其他插件后续安装的包装不能被覆盖。"""
        if self.wrapper is not None and NotificationMixin.post_message_strict is self.wrapper:
            NotificationMixin.post_message_strict = self.original
        self.original, self.wrapper = None, None
