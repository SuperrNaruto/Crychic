"""从原生通知提取显示字段，不保存原始 Message 或未知附件。"""

import hashlib
import json
import re
import uuid
from urllib.parse import parse_qs, unquote, urlsplit

MAX_TEXT = 16384
MAX_TITLE = 512
MAX_URL = 4096
SENSITIVE = re.compile(r"(?i)(api[_-]?key|token|cookie|passkey|password|authorization)\s*[:=]\s*\S+|[A-Za-z]:\\\S+")
# 原生整理通知以「源文件：/…」等标签给出路径，标签后整行可能含空格，因此整段隐藏。
LABELED_PATH = re.compile(r"(源文件|源目录|源路径|目标路径|目标目录|文件路径|路径)\s*[:：]\s*[^\n]+")
# 路径可紧贴中文或标点；字母数字后的「1/2/3」与 URL 中的「//」不构成路径起点。
POSIX_PATH = re.compile(r"(^|[^A-Za-z0-9_./-])(?:/[^\s/、，,;；)）\]】」\"']+){2,}/?", re.MULTILINE)
CHANNEL = re.compile(r"^@[A-Za-z0-9_]+$")
SENSITIVE_URL_PARTS = ("key", "token", "pass", "cookie", "authorization", "secret", "credential", "signature")
SENSITIVE_URL_KEYS = ("auth", "sig")


def redact_text(value, *, secrets=()):
    """去除已知凭据与敏感路径；读取旧队列时同样适用。"""
    text = str(value or "")
    for secret in secrets:
        if secret:
            text = text.replace(str(secret), "[已隐藏]")
    text = SENSITIVE.sub("[已隐藏]", text)
    text = LABELED_PATH.sub(r"\1：[已隐藏]", text)
    return POSIX_PATH.sub(r"\1[已隐藏]", text)


def safe_text(value, *, limit, secrets=()):
    """先去除已知凭据与敏感路径，再限定显示大小。"""
    text = redact_text(value, secrets=secrets)
    if len(text) > limit:
        raise ValueError("notice_too_large")
    return text


def decoded_url(raw):
    """展开百分号编码供安全检查，展示仍保留原始安全 URL。"""
    decoded = unquote(raw)
    while decoded != raw:
        raw, decoded = decoded, unquote(decoded)
    return decoded


def known_url_secret(raw, decoded, secrets):
    """已知凭据检查同时覆盖原始与编码后的地址。"""
    return any(str(secret) in decoded or str(secret) in raw for secret in secrets if secret)


def sensitive_url_fields(parts):
    """空敏感参数也拒绝，片段中的认证参数同样不能持久化。"""
    fields = parse_qs(parts.query, keep_blank_values=True)
    fields.update(parse_qs(parts.fragment, keep_blank_values=True))
    keys = [key.lower() for key in fields]
    return any(key in SENSITIVE_URL_KEYS or any(word in key for word in SENSITIVE_URL_PARTS) for key in keys)


def safe_url(value, *, secrets=()):
    """只保留无需凭据且长度有限的 HTTP 地址，不转发带密钥的图片或链接。"""
    raw = str(value or "")
    if len(raw) > MAX_URL:
        return ""
    decoded = decoded_url(raw)
    if known_url_secret(raw, decoded, secrets):
        return ""
    try:
        parts = urlsplit(raw)
        decoded_parts = urlsplit(decoded)
    except ValueError:
        return ""
    if parts.scheme not in ("http", "https") or not parts.hostname or parts.username is not None:
        return ""
    if sensitive_url_fields(decoded_parts):
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
                image=safe_url(message.image, secrets=secrets), link=safe_url(message.link, secrets=secrets))
