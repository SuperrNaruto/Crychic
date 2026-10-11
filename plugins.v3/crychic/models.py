"""插件自有持久队列模型，不依赖宿主消息表。"""

from sqlalchemy import BigInteger, Float, Integer, String, Text
from sqlalchemy.orm import Mapped, mapped_column
from app.sdk.database import plugin_declarative_base

Base = plugin_declarative_base()


class Progress(Base):
    """消息顺序、消费身份与已消费游标。"""

    __tablename__ = "progress"
    key: Mapped[str] = mapped_column(String, primary_key=True)
    channel: Mapped[str] = mapped_column(String)
    stream: Mapped[str] = mapped_column(String)
    head: Mapped[int] = mapped_column(BigInteger, default=0)
    consumed: Mapped[int] = mapped_column(BigInteger, default=0)
    consumer: Mapped[str] = mapped_column(String, default="")
    lease_until: Mapped[float] = mapped_column(Float, default=0)


class Pending(Base):
    """待消费的安全投影；消费确认后删除正文。"""

    __tablename__ = "pending"
    sequence: Mapped[int] = mapped_column(BigInteger, primary_key=True)
    identity: Mapped[str] = mapped_column(String, unique=True)
    payload: Mapped[str] = mapped_column(Text)


class Receipt(Base):
    """可信上游事件的最小去重身份，不保存正文。"""

    __tablename__ = "receipt"
    identity: Mapped[str] = mapped_column(String, primary_key=True)
    sequence: Mapped[int] = mapped_column(BigInteger)
