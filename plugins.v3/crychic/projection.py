"""从原生通知提取显示字段，不保存原始 Message 或未知附件。"""

import hashlib
import json
import re
import uuid
from urllib.parse import parse_qs, urlsplit

MAX_TEXT = 16384
MAX_TITLE = 512
SENSITIVE = re.compile(r"(?i)(api[_-]?key|token|cookie|passkey|password|authorization)\s*[:=]\s*\S+|(?:^|\s)(?:/[\w.-]+){2,}\S*|[A-Za-z]:\\\S+")
CHANNEL = re.compile(r"^@[A-Za-z0-9_]+$")


def safe_text(value, *, limit, secrets=()):
    """先去除已知凭据与敏感路径，再限定显示大小。"""
    text = str(value or "")
    for secret in secrets:
        if secret:
            text = text.replace(str(secret), "[已隐藏]")
    text = SENSITIVE.sub("[已隐藏]", text)
    if len(text) > limit:
        raise ValueError("notice_too_large")
    return text


def safe_url(value):
    """只保留无需凭据的 HTTP 地址，不转发带密钥的图片或链接。"""
    raw = str(value or "")
    parts = urlsplit(raw)
    if parts.scheme not in ("http", "https") or not parts.hostname or parts.username:
        return ""
    keys = [key.lower() for key in parse_qs(parts.query)]
    if any(any(word in key for word in ("key", "token", "pass", "cookie")) for key in keys):
        return ""
    return raw


def recipient(policy, message, *, default_target, topic):
    """使用原生已解析用户目标，默认目标只用于获准的默认范围。"""
    user = policy.get_notification_userid(message, "telegram_userid")
    if user:
        if not str(user).isdigit() or int(user) <= 0:
            raise ValueError("recipient_invalid")
        return dict(kind="personal", chat_id=str(user), user_id=str(user))
    if message.notification_route_scope == "user":
        raise ValueError("recipient_unbound")
    target = str(default_target or "")
    if target.isdigit() and int(target) > 0:
        return dict(kind="personal", chat_id=target, user_id=target)
    if not (target.startswith("-") and target[1:].isdigit()) and not CHANNEL.fullmatch(target):
        raise ValueError("default_recipient_missing")
    if topic and not target.startswith("-"):
        raise ValueError("topic_invalid")
    return dict(kind="channel", chat_id=target, topic_id=topic)


def project(message, target, *, channel, event_key=None, secrets=()):
    """生成平台中立的安全通知；业务状态和受众均不重新推断。"""
    text = message.text or message.rich_message or ""
    if target["kind"] == "channel" and message.username:
        text = str(text).replace(str(message.username), "[请求者已隐藏]")
    identity = uuid.uuid4().hex
    if event_key:
        material = json.dumps([event_key, channel, "telegram", target], sort_keys=True)
        identity = hashlib.sha256(material.encode()).hexdigest()
    return dict(id=identity, platform="telegram", recipient=target,
                title=safe_text(message.title, limit=MAX_TITLE, secrets=secrets),
                text=safe_text(text, limit=MAX_TEXT, secrets=secrets),
                image=safe_url(message.image), link=safe_url(message.link))
