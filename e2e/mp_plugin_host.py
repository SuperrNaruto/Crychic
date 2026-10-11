"""运行于官方 MP 镜像的隔离 E2E：真实宿主、真实插件和 HTTP fake。

不替换 MP 内部包，不连接真实服务；仅使用容器临时合成状态。
"""

import asyncio
import json
from pathlib import Path
import urllib.request
from datetime import datetime

import uvicorn
from app.factory import create_app
from app.application.configuration import get_configured_system_config
from app.application.plugin.runtime import get_plugin_manager
from app.application.plugin.routes import register_plugin_api
from app.schemas.types import SystemConfigKey, MessageType
from app.schemas.message import Message
from app.chain.system import SystemChain
from app.db.oper.plugininstance import PluginInstanceOper
from app.db.oper.user import UserOper
from app.runtime.settings import get_runtime_setting
from app.application.chain.context import get_chain_runtime_context
from app.runtime.extensions.module.manager import ModuleManager

PORT = 18181
ROOT = f"http://127.0.0.1:{PORT}/api/v1/plugin/Crychic/"
TOKEN = "crychic-fixture-api-key"
CONSUMER = "a" * 32
TRACE = []
DAY_HOURS = 24


def http_call(path, *, body=None):
    """走宿主真实插件认证和 HTTP 路由。"""
    data = None if body is None else json.dumps(body).encode()
    request = urllib.request.Request(ROOT + path, data=data,
                                    headers={"X-API-KEY": TOKEN, "Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=10) as response:
        value = json.load(response)
    if not value.get("success"):
        raise RuntimeError("fixture API rejected")
    return value["data"]


def setup_plugin():
    """用原生配置服务与插件管理器启动候选源码。"""
    config = get_configured_system_config()
    users = UserOper()
    name = get_runtime_setting("SUPERUSER")
    if users.get_by_name(name) is None:
        users.add(name=name, is_active=True, is_superuser=True)
    config.set(SystemConfigKey.Notifications, [dict(id="fixture-channel", name="FixtureCrychic",
               type="crychic", enabled=True, switchs=["插件"], config={})])
    config.set(SystemConfigKey.UserInstalledPlugins, ["Crychic"])
    oper = PluginInstanceOper()
    oper.save_config_data(instance_id="Crychic", source_plugin_id="Crychic", config_data=dict(enabled=True, default_target="1001", topic_id=0))
    oper.set_enabled(instance_id="Crychic", is_enabled=True)
    manager = get_plugin_manager()
    manager.classify_plugins()
    manager.start("Crychic")
    manager.init_plugin("Crychic", dict(enabled=True, default_target="1001", topic_id=0))
    register_plugin_api("Crychic")


async def exercise():
    """以真实业务发送入口驱动持久缓冲和 HTTP 确认。"""
    status = await asyncio.to_thread(http_call, "status")
    cursor = dict(consumer_id=CONSUMER, channel_id=status["channel_id"],
                  stream_id=status["stream_id"], sequence=status["head"])
    await asyncio.to_thread(http_call, "ack", body=cursor)
    chain = SystemChain()
    chain.post_message(Message(mtype=MessageType.Plugin, title="Native fixture", text="HTTP boundary"))
    query = "notifications?after=0&wait=0&consumer_id=" + CONSUMER
    query += "&channel_id=" + cursor["channel_id"] + "&stream_id=" + cursor["stream_id"]
    batch = await asyncio.to_thread(http_call, query)
    if len(batch["messages"]) != 1:
        raise RuntimeError("native provider was not invoked")
    item = batch["messages"][0]
    if item["recipient"]["chat_id"] != "1001" or item["title"] != "Native fixture":
        raise RuntimeError("native facts or recipient changed")
    TRACE.append("native route -> custom provider -> durable outbox -> authenticated HTTP")
    cursor["sequence"] = item["sequence"]
    await asyncio.to_thread(http_call, "ack", body=cursor)
    await asyncio.to_thread(http_call, "ack", body=cursor)
    TRACE.append("duplicate consumption ACK -> one committed checkpoint")
    await persistence_and_identity(chain, cursor)
    await failure_isolation(chain, cursor)
    await filtering_and_schedule(chain, cursor)
    if ModuleManager().get_running_module("TelegramModule") is not None:
        raise RuntimeError("plugin activated a Telegram bot")
    TRACE.append("custom channel -> no native Telegram client or update receiver")


def poll_query(cursor):
    """构造包含固定源身份的非破坏读取请求。"""
    return "notifications?after=" + str(cursor["sequence"]) + "&wait=0&consumer_id=" + CONSUMER + \
           "&channel_id=" + cursor["channel_id"] + "&stream_id=" + cursor["stream_id"]


async def persistence_and_identity(chain, cursor):
    """真实插件重新初始化、事务记录和 durable 事件身份。"""
    message = Message(mtype=MessageType.Plugin, title="Durable fixture", text="token=do-not-display")
    chain.post_message_strict(message, event_key="fixture-stable-event")
    chain.post_message_strict(message, event_key="fixture-stable-event")
    batch = await asyncio.to_thread(http_call, poll_query(cursor))
    if len(batch["messages"]) != 1:
        raise RuntimeError("durable replay duplicated notification")
    item = batch["messages"][0]
    if "do-not-display" in item["text"]:
        raise RuntimeError("unsafe original body persisted")
    get_plugin_manager().init_plugin("Crychic", dict(enabled=True, default_target="1001", topic_id=0))
    register_plugin_api("Crychic")
    resumed = await asyncio.to_thread(http_call, poll_query(cursor))
    if resumed["messages"] != batch["messages"]:
        raise RuntimeError("pending notification did not survive plugin restart")
    cursor["sequence"] = item["sequence"]
    await asyncio.to_thread(http_call, "ack", body=cursor)
    TRACE.append("plugin restart -> same durable payload; stable event replay -> one message")
    TRACE.append("safe projection -> no sensitive original value")


async def failure_isolation(chain, cursor):
    """插件先于系统渠道执行；捕获失败时 durable 调度不得被插件异常中断。"""
    config = get_configured_system_config()
    rows = config.get(SystemConfigKey.Notifications)
    config.set(SystemConfigKey.Notifications, rows + [dict(rows[0], id="fixture-second", name="SecondCrychic")])
    try:
        chain.post_message_strict(Message(mtype=MessageType.Plugin, title="Ambiguous", text="token=hidden"),
                                  event_key="fixture-ambiguous-event")
    finally:
        config.set(SystemConfigKey.Notifications, rows)
    isolated = await asyncio.to_thread(http_call, poll_query(cursor))
    if isolated["messages"]:
        raise RuntimeError("ambiguous channel queued a notification")
    TRACE.append("plugin capture failure -> strict native dispatch continues; nothing queued")


async def filtering_and_schedule(chain, cursor):
    """真实原生类型检查和时间队列，不伪造内部实现。"""
    config = get_configured_system_config()
    rows = config.get(SystemConfigKey.Notifications)
    rows[0]["switchs"] = []
    config.set(SystemConfigKey.Notifications, rows)
    chain.post_message(Message(mtype=MessageType.Plugin, title="Filtered", text="not sent"))
    filtered = await asyncio.to_thread(http_call, poll_query(cursor))
    if filtered["messages"]:
        raise RuntimeError("disabled native type forwarded")
    rows[0]["switchs"] = ["插件"]
    config.set(SystemConfigKey.Notifications, rows)
    hour = (datetime.now().hour + 1) % DAY_HOURS
    config.set(SystemConfigKey.NotificationSendTime, [dict(start=f"{hour:02d}:00", end=f"{hour:02d}:01")])
    queue = get_chain_runtime_context().message_queue
    queue.init_config()
    chain.post_message(Message(mtype=MessageType.Plugin, title="Scheduled", text="wait for native time"))
    scheduled = await asyncio.to_thread(http_call, poll_query(cursor))
    if scheduled["messages"]:
        raise RuntimeError("plugin bypassed native send-time queue")
    TRACE.append("native disabled type and send-time queue -> no premature delivery")
    config.set(SystemConfigKey.NotificationSendTime, [])
    queue.init_config()


async def main():
    """只启动隔离安全模式宿主，外部网络由容器禁用。"""
    app = create_app()
    async with app.router.lifespan_context(app):
        setup_plugin()
        server = uvicorn.Server(uvicorn.Config(app, host="127.0.0.1", port=PORT,
                                lifespan="off", log_level="error"))
        task = asyncio.create_task(server.serve())
        while not server.started:
            await asyncio.sleep(0.05)
        try:
            await exercise()
            print("CRYCHIC_PLUGIN_E2E " + json.dumps(TRACE))
        finally:
            server.should_exit = True
            await task


if __name__ == "__main__":
    asyncio.run(main())
