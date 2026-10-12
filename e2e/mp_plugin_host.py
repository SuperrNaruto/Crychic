"""运行于官方 MP 镜像的隔离 E2E：真实宿主、真实插件和 HTTP fake。

不替换 MP 内部包，不连接真实服务；仅使用容器临时合成状态。
"""

import asyncio
import json
import re
import threading
import time
from pathlib import Path
import urllib.request
from urllib.parse import urlencode
from datetime import datetime

import uvicorn
from sqlalchemy import event, text
from sqlalchemy.engine import Engine
from app.factory import create_app
from app.application.configuration import get_configured_system_config
from app.application.plugin.runtime import get_plugin_manager
from app.application.plugin.routes import register_plugin_api
from app.schemas.types import SystemConfigKey, MessageType, MediaType, ProgressKey
from app.schemas.message import Message
from app.chain.system import SystemChain
from app.db.oper.plugininstance import PluginInstanceOper
from app.db.oper.user import UserOper
from app.runtime.settings import get_runtime_setting
from app.application.chain.context import get_chain_runtime_context
from app.runtime.extensions.module.manager import ModuleManager
from app.startup.initializers.command import configure_command_runtime, stop_command
from app.chain.transfer.facade import TransferChain
from app.application.transfer.models import TransferTask
from app.application.transfer.feedback import TransferFailureNotification, render_transfer_failure_notification
from app.domain.context import MediaInfo
from app.domain.metainfo import MetaInfo
from app.runtime.progress import ProgressHelper
from app.schemas.file import FileItem
from app.modules.filemanager.storages import transfer_process

PORT = 18181
ROOT = f"http://127.0.0.1:{PORT}/api/v1/plugin/Crychic/"
PLUGIN_ROOT = f"http://127.0.0.1:{PORT}/api/v1/plugin/"
TOKEN = "crychic-fixture-api-key"
CONSUMER = "a" * 32
TRACE = []
DAY_HOURS = 24
HTTP_TIMEOUT = 10
IDLE_WAIT = 3
MAX_IDLE_READS = 3
MAX_IDLE_LEASE_UPDATES = 1
WAIT_SLACK = 0.5
POLL_COMPLETION_BUDGET = 2
LONG_WAIT = 25
FAILED_SOURCE = "/fixture-downloads"
FAILED_TARGET = "/fixture-library"


class SQLObservation:
    """只观察真实数据库开销，不替换宿主或插件执行。"""

    def __init__(self):
        self.reads = 0
        self.lease_updates = 0
        self.ready = threading.Event()
        self.lock = threading.Lock()

    def __enter__(self):
        event.listen(Engine, "after_cursor_execute", self.observe, named=True)
        return self

    def __exit__(self, *_):
        event.remove(Engine, "after_cursor_execute", self.observe)

    def observe(self, **kwargs):
        statement = " ".join(kwargs["statement"].lower().replace('"', '').split())
        with self.lock:
            if re.search(r"\bfrom progress\b", statement):
                self.reads += 1
                self.ready.set()
            if statement.startswith("update progress ") and "lease_until" in statement:
                self.lease_updates += 1


def http_reply(path, *, body=None, root=ROOT, method=None):
    """保留认证 HTTP 结果，用于核验明确拒绝。"""
    data = None if body is None else json.dumps(body).encode()
    request = urllib.request.Request(root + path, data=data, method=method,
                                    headers={"X-API-KEY": TOKEN, "Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=HTTP_TIMEOUT) as response:
        return json.load(response)


def http_call(path, *, body=None, root=ROOT):
    """走宿主真实插件认证和 HTTP 路由。"""
    value = http_reply(path, body=body, root=root)
    if not value.get("success"):
        raise RuntimeError("fixture API rejected")
    return value["data"]


def setup_plugin():
    """用原生配置服务与插件管理器启动候选源码。"""
    configure_command_runtime()
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
    await configuration_guidance()
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
    await safe_url_projection(chain, cursor)
    await transfer_failure_paths(chain, cursor)
    await idle_polling(cursor)
    await polling_lifecycle(chain, cursor)
    await failure_isolation(chain, cursor)
    await filtering_and_schedule(chain, cursor)
    await transfer_progress()
    if ModuleManager().get_running_module("TelegramModule") is not None:
        raise RuntimeError("plugin activated a Telegram bot")
    TRACE.append("custom channel -> no native Telegram client or update receiver")


def poll_query(cursor, *, wait=0, consumer=CONSUMER):
    """构造包含固定源身份的非破坏读取请求。"""
    return "notifications?" + urlencode(dict(after=cursor["sequence"], wait=wait, consumer_id=consumer,
           channel_id=cursor["channel_id"], stream_id=cursor["stream_id"]))


def form_fields(nodes):
    """遍历公开表单协议取得可配置字段，不读取内部实现。"""
    fields = {}
    for node in nodes:
        props = node.get("props", {})
        if props.get("model"):
            fields[props["model"]] = props
        fields.update(form_fields(node.get("content", [])))
    return fields


async def read_guidance():
    """读取宿主真实配置及详情端点，检查渲染模式与敏感信息。"""
    saved = dict(get_plugin_manager().get_plugin_config("Crychic"))
    channels = json.dumps(get_configured_system_config().get(SystemConfigKey.Notifications))
    form = await asyncio.to_thread(http_call, "form/Crychic", root=PLUGIN_ROOT)
    page = await asyncio.to_thread(http_call, "page/Crychic", root=PLUGIN_ROOT)
    if get_plugin_manager().get_plugin_config("Crychic") != saved:
        raise RuntimeError("UI reads changed saved plugin parameters")
    if json.dumps(get_configured_system_config().get(SystemConfigKey.Notifications)) != channels:
        raise RuntimeError("UI reads created or changed native notification channels")
    if form.get("render_mode") != "vuetify" or not form.get("conf") or not page.get("page"):
        raise RuntimeError("guided configuration did not reach the public host UI contract")
    if TOKEN in json.dumps([form, page]):
        raise RuntimeError("configuration guidance exposed an API credential")
    return form


def channel_values(form):
    """从宿主公开表单取得选择所绑定的稳定渠道值。"""
    field = form_fields(form["conf"])["channel_id"]
    return {item["value"] for item in field.get("items", [])}


def source_progress(status):
    """只比较公开协议的来源和持久进度，忽略无关状态扩展。"""
    return tuple(status[key] for key in ("channel_id", "stream_id", "head"))


async def configuration_guidance():
    """单条、多条及空渠道目录通过认证 HTTP 验证，原设置最终恢复。"""
    config = get_configured_system_config()
    rows = config.get(SystemConfigKey.Notifications)
    original = get_plugin_manager().get_plugin_config("Crychic")
    before = await asyncio.to_thread(http_call, "status")
    form = await read_guidance()
    if form["model"].get("default_target") != "1001" or not form["model"].get("enabled"):
        raise RuntimeError("opening guided form replaced the saved configuration")
    if channel_values(form) != {"", "fixture-channel"}:
        raise RuntimeError("guided form does not expose native channel choices")
    try:
        extra = [dict(rows[0], id="fixture-second", name="SecondCrychic"),
                 dict(rows[0], id="fixture-other", name="OtherChannel", type="fixture-other", enabled=False)]
        config.set(SystemConfigKey.Notifications, rows + extra)
        multiple = await read_guidance()
        if channel_values(multiple) != {"", "fixture-channel", "fixture-second"}:
            raise RuntimeError("multiple native channels cannot be explicitly selected")
        result = await asyncio.to_thread(http_reply, "Crychic", root=PLUGIN_ROOT, method="PUT",
                  body=dict(original, channel_id="fixture-channel"))
        selected = await read_guidance()
        if not result.get("success") or selected["model"].get("channel_id") != "fixture-channel":
            raise RuntimeError("selected channel ID was not saved and reloaded")
        config.set(SystemConfigKey.Notifications, [])
        empty = await read_guidance()
        if not form_fields(empty["conf"])["channel_id"].get("disabled"):
            raise RuntimeError("missing channel directory offered a misleading choice")
    finally:
        config.set(SystemConfigKey.Notifications, rows)
        await asyncio.to_thread(http_reply, "Crychic", root=PLUGIN_ROOT, method="PUT", body=original)
    after = await asyncio.to_thread(http_call, "status")
    if source_progress(after) != source_progress(before) or get_plugin_manager().get_plugin_config("Crychic") != original:
        raise RuntimeError("reading configuration guidance changed queue progress or settings")
    TRACE.append("guided host form -> native channel choices; explicit selection saves and reloads")
    TRACE.append("guided UI reads -> saved settings preserved; no notification, reset or credential disclosure")


async def safe_url_projection(chain, cursor):
    """认证读取及重载证明 URL 安全边界，正常 URL 为配对正例。"""
    image, link = "https://images.example.invalid/poster.jpg", "https://media.example.invalid/view"
    encoded = "".join(f"%{ord(char):02X}" for char in TOKEN)
    cases = [(image, link, image, link),
             (image + "?auth=" + TOKEN, link + "?v=" + encoded, "", ""),
             (image + "/" + encoded, link + "#" + TOKEN, "", ""),
             (image + "?%61uthorization=fixture", link + "?signature=fixture", "", ""),
             (image + "?token=", link + "?cookie=", "", ""),
             ("http://[invalid", "http://[invalid", "", "")]
    for index, (source_image, source_link, expected_image, expected_link) in enumerate(cases):
        title = f"URL fixture {index}"
        chain.post_message_strict(Message(mtype=MessageType.Plugin, title=title, text="safe body",
                                  image=source_image, link=source_link), event_key=f"fixture-url-{index}")
        verify_stored_urls((expected_image, expected_link))
        if source_image != expected_image:
            seed_legacy_urls(source_image, source_link)
        batch = await asyncio.to_thread(http_call, poll_query(cursor))
        item = verify_url_item(batch, title=title, expected=(expected_image, expected_link))
        if source_image != expected_image:
            await verify_url_restart(cursor, batch)
        cursor["sequence"] = item["sequence"]
        await asyncio.to_thread(http_call, "ack", body=cursor)
    TRACE.append("URL projection and restart -> safe URLs kept; raw/encoded secrets and malformed URLs removed")
    TRACE.append("legacy durable URL fixture -> sanitized in storage; identity and native facts retained")


def verify_url_item(batch, *, title, expected):
    """正文、正常 URL 与持久化修正分别有独立期望。"""
    if len(batch["messages"]) != 1:
        raise RuntimeError("URL projection lost the native notification")
    item = batch["messages"][0]
    if (item["image"], item["link"]) != expected:
        raise RuntimeError("URL safe projection boundary failed")
    if item["title"] != title or item["text"] != "safe body":
        raise RuntimeError("URL filtering changed native facts")
    verify_stored_urls(expected)
    return item


def verify_stored_urls(expected):
    """首次落盘及旧队列读取后均直接核对持久记录。"""
    stored = persisted_payload()
    if (stored["image"], stored["link"]) != expected:
        raise RuntimeError("URL credential crossed the durable storage boundary")


async def verify_url_restart(cursor, batch):
    """修正后的同一队列内容重载后保持安全，不换事件身份。"""
    get_plugin_manager().init_plugin("Crychic", dict(enabled=True, default_target="1001", topic_id=0))
    register_plugin_api("Crychic")
    resumed = await asyncio.to_thread(http_call, poll_query(cursor))
    if resumed["messages"] != batch["messages"]:
        raise RuntimeError("unsafe URL survived only in persistent state")


def persisted_payload():
    """只读取隔离插件自有数据库中的合成旧状态。"""
    database = get_plugin_manager().get_plugin_attr("Crychic", "get_database")()
    with database.session() as session:
        rows = session.execute(text("SELECT payload FROM pending")).scalars().all()
    if len(rows) != 1:
        raise RuntimeError("legacy fixture did not reach durable storage")
    return json.loads(rows[0])


def seed_legacy_urls(image, link):
    """用真实 SDK 数据库模拟旧版本持久记录，不替换内部执行路径。"""
    payload = dict(persisted_payload(), image=image, link=link)
    database = get_plugin_manager().get_plugin_attr("Crychic", "get_database")()
    with database.session() as session, session.begin():
        session.execute(text("UPDATE pending SET payload=:payload"),
                        dict(payload=json.dumps(payload, ensure_ascii=False)))


def failure_notices():
    """用宿主自己的整理失败渲染器生成单文件与多文件正文，路径均为合成值。"""
    def notice(name):
        return TransferFailureNotification(
            media_title="Fixture Show", season_episode="S01E02", reason="目标已存在", history_id=7,
            image=None, username=None, source_path=f"{FAILED_SOURCE}/Fixture Show (2024)/{name}",
            target_path=f"{FAILED_TARGET}/Fixture Show")
    return [render_transfer_failure_notification([notice("a.mkv")]),
            render_transfer_failure_notification([notice("a.mkv"), notice("b.mkv")])]


def verify_failure_item(item, title):
    """标题与非路径事实保留，HTTP 与持久记录都不含路径。"""
    stored = persisted_payload()
    for value in (item["text"], stored["text"]):
        if FAILED_SOURCE in value or FAILED_TARGET in value:
            raise RuntimeError("transfer failure path crossed the plugin boundary")
    if item["title"] != title or "目标已存在" not in item["text"]:
        raise RuntimeError("path hiding changed native facts")


async def transfer_failure_paths(chain, cursor):
    """原生整理失败通知的路径紧贴全角冒号；旧队列中的路径读取时修正。"""
    for index, (title, body) in enumerate(failure_notices()):
        chain.post_message_strict(Message(mtype=MessageType.Plugin, title=title, text=body),
                                  event_key=f"fixture-failure-{index}")
        if FAILED_SOURCE in persisted_payload()["text"]:
            raise RuntimeError("transfer failure path entered durable storage")
        database = get_plugin_manager().get_plugin_attr("Crychic", "get_database")()
        with database.session() as session, session.begin():
            session.execute(text("UPDATE pending SET payload=:payload"),
                            dict(payload=json.dumps(dict(persisted_payload(), text=body), ensure_ascii=False)))
        batch = await asyncio.to_thread(http_call, poll_query(cursor))
        item = batch["messages"][0]
        verify_failure_item(item, title)
        cursor["sequence"] = item["sequence"]
        await asyncio.to_thread(http_call, "ack", body=cursor)
    TRACE.append("native transfer failure paths -> hidden at capture; legacy stored paths hidden on read")


async def idle_polling(cursor):
    """三秒真实 HTTP 等待只观察数据库，不用请求次数冒充数据库预算。"""
    started = time.monotonic()
    with SQLObservation() as observation:
        batch = await asyncio.to_thread(http_call, poll_query(cursor, wait=IDLE_WAIT))
    elapsed = time.monotonic() - started
    print("CRYCHIC_PLUGIN_METRIC " + json.dumps(dict(idle_seconds=IDLE_WAIT,
          progress_reads=observation.reads, lease_updates=observation.lease_updates)))
    if batch["messages"] or observation.reads == 0:
        raise RuntimeError("idle database observation did not exercise empty long polling")
    if observation.reads > MAX_IDLE_READS or observation.lease_updates > MAX_IDLE_LEASE_UPDATES:
        raise RuntimeError("idle long polling exceeds database budget")
    if elapsed < IDLE_WAIT - WAIT_SLACK or elapsed > IDLE_WAIT + POLL_COMPLETION_BUDGET:
        raise RuntimeError("idle long polling did not respect the real wait deadline")
    TRACE.append("idle long poll -> bounded database reads and lease writes; real wait deadline retained")


async def start_observed_poll(cursor, observation):
    """真实 SQL 已开始后再注入通知或重载，避免依赖碰巧休眠。"""
    task = asyncio.create_task(asyncio.to_thread(http_reply, poll_query(cursor, wait=LONG_WAIT)))
    ready = await asyncio.to_thread(observation.ready.wait, HTTP_TIMEOUT)
    if not ready:
        raise RuntimeError("long poll did not reach the database boundary")
    return task


async def polling_lifecycle(chain, cursor):
    """原生入队唤醒等待，排他租约与重载隔离均从 HTTP 核验。"""
    with SQLObservation() as observation:
        task = await start_observed_poll(cursor, observation)
        foreign = await asyncio.to_thread(http_reply, poll_query(cursor, consumer="b" * len(CONSUMER)))
        if foreign.get("success") is not False:
            raise RuntimeError("active long poll allowed another consumer to take over")
        chain.post_message_strict(Message(mtype=MessageType.Plugin, title="Wake fixture", text="native wake"),
                                  event_key="fixture-wake")
        reply = await asyncio.wait_for(task, timeout=POLL_COMPLETION_BUDGET)
    if not reply.get("success") or len(reply["data"]["messages"]) != 1:
        raise RuntimeError("native enqueue did not wake the active long poll")
    cursor["sequence"] = reply["data"]["messages"][0]["sequence"]
    await asyncio.to_thread(http_call, "ack", body=cursor)
    TRACE.append("native enqueue -> waiting HTTP wakes; competing consumer rejected")
    with SQLObservation() as observation:
        task = await start_observed_poll(cursor, observation)
        get_plugin_manager().init_plugin("Crychic", dict(enabled=True, default_target="1001", topic_id=0))
        register_plugin_api("Crychic")
        reply = await asyncio.wait_for(task, timeout=POLL_COMPLETION_BUDGET)
    if reply.get("success") is not False:
        raise RuntimeError("old long poll survived plugin generation change")
    current = await asyncio.to_thread(http_call, poll_query(cursor))
    if current["messages"] or current["stream_id"] != cursor["stream_id"]:
        raise RuntimeError("plugin reload changed persistent source or consumed pending facts")
    TRACE.append("plugin reload -> old HTTP wait ends; persistent source and checkpoint retained")


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


FIXTURE_DIR = "/fixture/downloads/Fixture.Show.S01"
FIXTURE_HASH = "c" * 40
FIXTURE_TMDB = 274580
RUNNING_PERCENT = 37.5
FULL_PERCENT = 100
GLOBAL_PERCENT = 80


def transfer_task(episode):
    """真实宿主任务模型；路径只在宿主内存中，用于证明投影不外泄。"""
    name = f"Fixture.Show.S01E{episode:02d}.1080p.mkv"
    media = MediaInfo()
    media.type, media.tmdb_id, media.title = MediaType.TV, FIXTURE_TMDB, "Fixture Show"
    media.media_source, media.media_id = "themoviedb", str(FIXTURE_TMDB)
    return TransferTask(fileitem=FileItem(storage="local", type="file", path=f"{FIXTURE_DIR}/{name}", name=name),
                        meta=MetaInfo(name), mediainfo=media, downloader="qBittorrent", download_hash=FIXTURE_HASH)


async def transfer_progress():
    """真实作业视图与宿主上传进度写入方：只投影有记录的运行中文件，全局进度和路径不外泄。"""
    jobs = TransferChain().jobview
    running, waiting, unrecorded = transfer_task(13), transfer_task(12), transfer_task(14)
    for task in (running, waiting, unrecorded):
        jobs.add_task(task)
    jobs.running_task(running)
    jobs.running_task(unrecorded)
    total = ProgressHelper(ProgressKey.FileTransfer)
    total.start()
    total.update(value=GLOBAL_PERCENT, text=f"正在整理 {running.fileitem.path}")
    report = transfer_process(Path(running.fileitem.path).as_posix())
    report(RUNNING_PERCENT)
    try:
        data = await asyncio.to_thread(http_call, "transfer-progress")
        expected = dict(media_source="themoviedb", media_id=str(FIXTURE_TMDB), season=1, begin_episode=13,
                        end_episode=None, download_hash=FIXTURE_HASH, downloader="qBittorrent",
                        active=True, percent=RUNNING_PERCENT)
        if data["files"] != [expected]:
            raise RuntimeError("transfer progress projection changed: " + json.dumps(data["files"]))
        if FIXTURE_DIR in json.dumps(data) or "mkv" in json.dumps(data):
            raise RuntimeError("transfer progress exposed a path or file name")
        TRACE.append("running queue file -> authenticated progress by identity; waiting, unrecorded and global excluded")
        report(FULL_PERCENT)
        ended = await asyncio.to_thread(http_call, "transfer-progress")
        if [(f["active"], f["percent"]) for f in ended["files"]] != [(False, 100.0)]:
            raise RuntimeError("finished file progress was not reported as ended")
        TRACE.append("finished file progress -> inactive full value; no path or file name")
    finally:
        total.end()
        for task in (running, waiting, unrecorded):
            jobs.remove_task(task.fileitem)


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
            stop_command()


if __name__ == "__main__":
    asyncio.run(main())
