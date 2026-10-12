"""插件自有事务队列；同一实例的写入在锁下串行。"""

import json
import threading
import time
import uuid
from sqlalchemy import delete, select
from .models import Pending, Progress, Receipt
from .projection import redact_text, safe_url

PROTOCOL = 1
META_KEY = "native"
LEASE_SECONDS = 60
LEASE_RENEW_MARGIN = LEASE_SECONDS / 2
MAX_BATCH = 32


class Outbox:
    """只通过宿主提供的插件数据库会话访问自有表。"""

    def __init__(self, database, *, secrets=(), lock=None):
        """保存数据库取得函数，初始化阶段不访问尚未建立的表。"""
        self.database = database
        self.secrets = secrets
        self.lock = lock if lock is not None else threading.RLock()
        self.changed = threading.Condition()
        self.revision = 0
        self.closed = False

    def version(self):
        """读取通知代次，先于数据库快照取得以防遗漏并发入队。"""
        with self.changed:
            return self.revision

    def wait(self, version, timeout):
        """等待入队或停用；独立条件锁不阻塞数据库事务与宿主事件循环。"""
        with self.changed:
            changed = self.changed.wait_for(lambda: self.closed or self.revision != version, timeout)
            if self.closed:
                raise ValueError("inactive")
            return changed

    def close(self):
        """唤醒旧代等待，不清空队列、不关闭数据库引擎。"""
        with self.changed:
            self.closed = True
            self.changed.notify_all()

    def metadata(self, session, channel):
        """绑定稳定渠道身份，来源变化必须明确恢复而非自动重放。"""
        meta = session.get(Progress, META_KEY)
        if meta is None:
            meta = Progress(key=META_KEY, channel=channel, stream=uuid.uuid4().hex,
                            head=0, consumed=0, consumer="", lease_until=0)
            session.add(meta)
            session.flush()
        if meta.channel != channel:
            raise ValueError("channel_changed")
        return meta

    def status(self, channel):
        """返回无正文的协议与进度快照。"""
        with self.lock, self.database().session() as session, session.begin():
            return self.status_data(self.metadata(session, channel))

    @staticmethod
    def status_data(meta):
        """构造只含必要身份和顺序的返回值。"""
        return dict(protocol=PROTOCOL, channel_id=meta.channel, stream_id=meta.stream,
                    head=meta.head, active=True)

    def append(self, channel, payload):
        """先事务提交安全投影，再允许消费者读取。"""
        with self.lock, self.database().session() as session, session.begin():
            meta = self.metadata(session, channel)
            identity = payload["id"]
            if session.get(Receipt, identity) is not None:
                return
            meta.head += 1
            payload = dict(payload, sequence=meta.head)
            session.add(Pending(sequence=meta.head, identity=identity,
                                payload=json.dumps(payload, ensure_ascii=False)))
            session.add(Receipt(identity=identity, sequence=meta.head))
        with self.changed:
            self.revision += 1
            self.changed.notify_all()

    @staticmethod
    def claim(meta, query):
        """固定来源与排他租约，确认重复调用保持幂等。"""
        if query.get("channel_id") != meta.channel or query.get("stream_id") != meta.stream:
            raise ValueError("source_changed")
        consumer = query.get("consumer_id", "")
        if not isinstance(consumer, str) or len(consumer) != 32:
            raise ValueError("consumer_invalid")
        now = time.time()
        if meta.consumer not in ("", consumer) and meta.lease_until > now:
            raise ValueError("consumer_conflict")
        if meta.consumer != consumer or meta.lease_until <= now + LEASE_RENEW_MARGIN:
            meta.consumer, meta.lease_until = consumer, now + LEASE_SECONDS

    def safe_item(self, row):
        """读取时修正旧队列的路径与 URL，保留事件身份、顺序与原生业务事实。"""
        item = json.loads(row.payload)
        for field in ("title", "text"):
            item[field] = redact_text(item.get(field), secrets=self.secrets)
        for field in ("image", "link"):
            item[field] = safe_url(item.get(field), secrets=self.secrets)
        payload = json.dumps(item, ensure_ascii=False)
        if payload != row.payload:
            row.payload = payload
        return item

    def read(self, channel, query):
        """非破坏读取完整连续范围，不在读取时删除记录。"""
        with self.lock, self.database().session() as session, session.begin():
            meta = self.metadata(session, channel)
            self.claim(meta, query)
            after = int(query["after"])
            if after < meta.consumed or after > meta.head:
                raise ValueError("cursor_invalid")
            statement = select(Pending).where(Pending.sequence > after).order_by(Pending.sequence).limit(MAX_BATCH)
            items = [self.safe_item(row) for row in session.scalars(statement)]
            expected = after
            for item in items:
                expected += 1
                if item["sequence"] != expected:
                    raise ValueError("range_incomplete")
            if not items and after < meta.head:
                raise ValueError("range_incomplete")
            return dict(self.status_data(meta), messages=items)

    def ack(self, channel, query):
        """确认消费并原子清理正文；不宣称平台已发送成功。"""
        with self.lock, self.database().session() as session, session.begin():
            meta = self.metadata(session, channel)
            self.claim(meta, query)
            sequence = int(query["sequence"])
            if sequence < meta.consumed or sequence > meta.head:
                raise ValueError("ack_invalid")
            meta.consumed = sequence
            session.execute(delete(Pending).where(Pending.sequence <= sequence))
            return dict(sequence=sequence)
