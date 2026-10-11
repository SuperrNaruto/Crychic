"""整理队列运行中文件的进度只读投影。

原生 `/system/progress/{key}` 只接受资源 Cookie，API key 无法稳定读取；公开 SDK 也没有
整理队列入口。这里只读 `/transfer/queue` 的同一来源（单例 `TransferChain` 的作业视图）与
进度缓存，键规则沿用写入方 `transfer_process`：
`md5(Path(fileitem.path).as_posix())`。不修改队列或进度，不返回路径、文件名与进度文本。
"""

from pathlib import Path

from app.chain.transfer.facade import TransferChain
from app.foundation.crypto import HashUtils
from app.runtime.progress import ProgressHelper

RUNNING = "running"
# 与原生整理队列页面同时订阅的文件上限一致。
MAX_FILES = 12
FULL = 100.0


def running_progress():
    """运行中文件的身份与进度；没有进度记录的文件不出现。"""
    files = []
    for job in TransferChain().get_queue_tasks():
        for task in list(job.tasks or []):
            if len(files) >= MAX_FILES:
                return files
            item = file_progress(job, task)
            if item is not None:
                files.append(item)
    return files


def file_progress(job, task):
    """单个运行中文件的安全投影；身份不全或进度不可用时为 None。"""
    path = getattr(task.fileitem, "path", None) if task.fileitem else None
    if task.state != RUNNING or not path:
        return None
    detail = ProgressHelper(HashUtils.md5(Path(path).as_posix())).get()
    value = detail.get("value") if isinstance(detail, dict) else None
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    media = job.media
    meta = task.meta
    return dict(
        media_source=getattr(media, "media_source", None),
        media_id=str(getattr(media, "media_id", None) or "") or None,
        season=job.season,
        begin_episode=getattr(meta, "begin_episode", None),
        end_episode=getattr(meta, "end_episode", None),
        download_hash=task.download_hash,
        downloader=task.downloader,
        active=bool(detail.get("enable")),
        percent=max(min(float(value), FULL), 0.0),
    )
