#!/usr/bin/env python3
"""SSHZYU asynchronous video generation, polling, and download CLI."""

from __future__ import annotations

import argparse
from dataclasses import dataclass, field
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import sys
import time
import urllib.parse
import uuid

sys.path.insert(0, str(Path(__file__).resolve().parent))
image_scripts = Path(__file__).resolve().parents[2] / "sshzyu-image" / "scripts"
if not (image_scripts / "sshzyu_image.py").is_file():
    raise SystemExit("缺少同一安装根下的 sshzyu-image；视频任务不能继续。")
sys.path.insert(1, str(image_scripts))

from media_common import (
    MediaError,
    classify_model,
    download_file,
    media_tools_status,
    probe_media,
    redact_secret,
    safe_filename,
    validate_aspect_ratio,
    validate_model_id,
    validate_reference_url,
    validate_resolution,
    write_json,
)
# 图片请求由独立 sshzyu-image 技能通过 sshzyu_image 兼容桥提供；本模块不实现图片 API。
from sshzyu_image import (
    BASE_URL,
    clean_key,
    request_json,
)


# 视频与图片分属不同的平台分组：/v1/videos 只接受 grok/composite/video 组，
# /v1/images/generations 只接受 openai/grok 组。同一把 Key 无法同时满足两者，
# 因此视频只读取独立凭据；缺失时直接失败并提示换 Key，不回退到生图 Key。
VIDEO_ENV_KEY = "SSHZYU_VIDEO_API_KEY"
VIDEO_ENV_FILE = Path(__file__).resolve().parents[1] / ".env"

MAX_PROMPT = 32000
MAX_SECONDS = 60
MAX_POLL_SECONDS = 6 * 60 * 60
TERMINAL_SUCCESS = {"completed", "complete", "succeeded", "success", "done"}
TERMINAL_FAILURE = {"failed", "failure", "error", "cancelled", "canceled", "expired"}
PENDING_STATUSES = {"queued", "pending", "in_progress", "processing", "running"}


def _read_env_value(env_file, name):
    path = Path(env_file).expanduser()
    if not path.exists():
        return None
    value = None
    for raw_line in path.read_text(encoding="utf-8-sig").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].lstrip()
        if "=" not in line:
            continue
        key_name, raw_value = line.split("=", 1)
        if key_name.strip() != name:
            continue
        raw_value = raw_value.strip()
        if len(raw_value) >= 2 and raw_value[0] == raw_value[-1] and raw_value[0] in ("'", '"'):
            raw_value = raw_value[1:-1]
        value = clean_key(raw_value)
    return value


def load_video_key(env_file):
    """加载视频专用凭据，返回 (key, source, used_generic_fallback)。

    视频组与生图组平台不同，通用 Key 通常属于 openai 生图组，
    用它调 /v1/videos 会被网关判为不支持并返回 404。
    因此不回退到通用 SSHZYU_API_KEY：找不到专用 Key 时返回 (None, "", False)，
    由调用方报错并提示用户换 Key，而不是拿生图 Key 去试。
    """
    env_path = Path(env_file).expanduser() if env_file else VIDEO_ENV_FILE
    from_env_file = _read_env_value(env_path, VIDEO_ENV_KEY)
    if from_env_file:
        return from_env_file, f"env:{env_path}", False
    from_process = os.environ.get(VIDEO_ENV_KEY)
    if from_process:
        return clean_key(from_process), f"process environment ({VIDEO_ENV_KEY})", False
    return None, "", False


class VideoError(MediaError):
    pass


class VideoTaskFailed(VideoError):
    pass


class VideoTaskTimeout(VideoError):
    pass


@dataclass
class VideoJob:
    id: str
    prompt: str
    model: str
    seconds: int
    resolution: object = None
    aspect_ratio: object = None
    image_urls: list = field(default_factory=list)
    video_urls: list = field(default_factory=list)
    audio_urls: list = field(default_factory=list)
    references: list = field(default_factory=list)
    timeout: float = 600
    poll_interval: float = 5
    poll_timeout: float = 1800

    @property
    def endpoint(self):
        return "/videos"


def emit(message, error=False):
    print(message, file=sys.stderr if error else sys.stdout, flush=True)


def _normalize_url_list(values, field_name):
    if values is None:
        return []
    if isinstance(values, str):
        values = [values]
    if not isinstance(values, list):
        raise VideoError(f"{field_name} 必须是字符串数组。")
    return [
        validate_reference_url(value, field_name)
        for value in values
    ]


def validate_references(references):
    if references is None:
        return []
    if not isinstance(references, list):
        raise VideoError("references 必须是对象数组。")
    normalized = []
    for index, item in enumerate(references, 1):
        if not isinstance(item, dict):
            raise VideoError(f"references[{index}] 必须是对象。")
        unknown = set(item) - {"type", "role", "url"}
        if unknown:
            raise VideoError(
                f"references[{index}] 包含未知字段: {', '.join(sorted(unknown))}"
            )
        ref_type = str(item.get("type") or "").strip().lower()
        if ref_type not in ("image", "video", "audio"):
            raise VideoError(f"references[{index}].type 必须是 image、video 或 audio。")
        role = str(item.get("role") or "reference").strip()
        if not role or len(role) > 80 or any(ord(char) < 32 for char in role):
            raise VideoError(f"references[{index}].role 无效。")
        normalized.append(
            {
                "type": ref_type,
                "role": role,
                "url": validate_reference_url(
                    item.get("url"),
                    f"references[{index}].url",
                ),
            }
        )
    return normalized


def validate_video_job(job):
    if not isinstance(job, VideoJob):
        raise VideoError("视频任务类型无效。")
    job.id = safe_filename(job.id)
    if not job.prompt or not job.prompt.strip():
        raise VideoError(f"{job.id}: prompt 不能为空。")
    if len(job.prompt) > MAX_PROMPT:
        raise VideoError(f"{job.id}: prompt 超过 {MAX_PROMPT} 字符。")
    try:
        job.model = validate_model_id(job.model)
    except MediaError as exc:
        raise VideoError(str(exc)) from None
    if not 1 <= int(job.seconds) <= MAX_SECONDS:
        raise VideoError(f"{job.id}: seconds 必须在 1 到 {MAX_SECONDS} 之间。")
    job.seconds = int(job.seconds)
    try:
        job.resolution = validate_resolution(job.resolution)
        job.aspect_ratio = validate_aspect_ratio(job.aspect_ratio)
        job.image_urls = _normalize_url_list(job.image_urls, "image_urls")
        job.video_urls = _normalize_url_list(job.video_urls, "video_urls")
        job.audio_urls = _normalize_url_list(job.audio_urls, "audio_urls")
        job.references = validate_references(job.references)
    except MediaError as exc:
        raise VideoError(str(exc)) from None
    if job.references and (job.image_urls or job.video_urls or job.audio_urls):
        raise VideoError(
            f"{job.id}: references 与 image_urls/video_urls/audio_urls 不能混用。"
        )
    if len(job.image_urls) > 30 or len(job.video_urls) > 10 or len(job.audio_urls) > 10:
        raise VideoError(f"{job.id}: 参考素材数量超过接口限制。")
    if len(job.references) > 30:
        raise VideoError(f"{job.id}: references 最多 30 项。")
    if job.timeout <= 0 or job.timeout > 1800:
        raise VideoError(f"{job.id}: 提交 timeout 必须大于 0 且不超过 1800 秒。")
    if job.poll_interval <= 0 or job.poll_interval > 300:
        raise VideoError(f"{job.id}: poll_interval 必须大于 0 且不超过 300 秒。")
    if job.poll_timeout <= 0 or job.poll_timeout > MAX_POLL_SECONDS:
        raise VideoError(
            f"{job.id}: poll_timeout 必须大于 0 且不超过 {MAX_POLL_SECONDS} 秒。"
        )
    return job


def build_video_payload(job):
    validate_video_job(job)
    payload = {
        "model": job.model,
        "prompt": job.prompt,
        "seconds": job.seconds,
    }
    if job.resolution:
        payload["resolution"] = job.resolution
    if job.aspect_ratio:
        payload["aspect_ratio"] = job.aspect_ratio
    if job.image_urls:
        payload["image_urls"] = list(job.image_urls)
    if job.video_urls:
        payload["video_urls"] = list(job.video_urls)
    if job.audio_urls:
        payload["audio_urls"] = list(job.audio_urls)
    if job.references:
        payload["references"] = list(job.references)
    return payload


VIDEO_JOB_KEYS = {
    "id",
    "prompt",
    "prompt_file",
    "model",
    "seconds",
    "duration",
    "resolution",
    "aspect_ratio",
    "image_urls",
    "video_urls",
    "audio_urls",
    "references",
    "timeout",
    "poll_interval",
    "poll_timeout",
}


def _positive_float(value, field_name, default):
    if value is None:
        return default
    try:
        parsed = float(value)
    except (TypeError, ValueError):
        raise VideoError(f"{field_name} 必须是数字。") from None
    return parsed


def merge_video_job(index, raw, defaults, overrides, base_dir, timeout):
    if isinstance(raw, str):
        raw = {"prompt": raw}
    if not isinstance(raw, dict):
        raise VideoError(f"任务 {index} 必须是字符串或对象。")
    unknown = set(raw) - VIDEO_JOB_KEYS
    if unknown:
        raise VideoError(
            f"任务 {index} 包含未知字段: {', '.join(sorted(unknown))}"
        )
    base_dir = Path(base_dir).expanduser().resolve()
    merged = {
        "id": f"shot-{index:02d}",
        "prompt": "",
        "model": "",
        "seconds": None,
        "resolution": None,
        "aspect_ratio": None,
        "image_urls": [],
        "video_urls": [],
        "audio_urls": [],
        "references": [],
        "timeout": timeout,
        "poll_interval": 5,
        "poll_timeout": 1800,
    }
    for source in (defaults, overrides, raw):
        for key, value in source.items():
            if key == "prompt_file":
                continue
            if key == "duration":
                if "seconds" in raw and value is not None and raw.get("seconds") != value:
                    raise VideoError(
                        f"任务 {index}: seconds 与 duration 同时填写时必须一致。"
                    )
                merged["seconds"] = value
            else:
                merged[key] = value
    prompt_file = raw.get("prompt_file")
    if prompt_file:
        prompt_path = Path(base_dir, prompt_file).expanduser().resolve()
        if not prompt_path.is_file():
            raise VideoError(f"任务 {index}: prompt_file 不存在: {prompt_path}")
        merged["prompt"] = prompt_path.read_text(encoding="utf-8-sig")
    if merged["seconds"] is None:
        raise VideoError(f"任务 {index}: 缺少 seconds 或 duration。")
    try:
        seconds = int(merged["seconds"])
    except (TypeError, ValueError):
        raise VideoError(f"任务 {index}: seconds 必须是整数。") from None
    merged["seconds"] = seconds
    merged["timeout"] = _positive_float(merged["timeout"], "timeout", timeout)
    merged["poll_interval"] = _positive_float(
        merged["poll_interval"], "poll_interval", 5
    )
    merged["poll_timeout"] = _positive_float(
        merged["poll_timeout"], "poll_timeout", 1800
    )
    try:
        job = VideoJob(**merged)
    except TypeError as exc:
        raise VideoError(f"任务 {index} 字段无效: {redact_secret(exc)}") from None
    return validate_video_job(job)


def print_video_plan(job):
    prompt_hash = hashlib.sha256(job.prompt.encode("utf-8")).hexdigest()[:12]
    emit(
        "VIDEO PLAN: "
        f"id={job.id}; endpoint={job.endpoint}; model={job.model}; "
        f"seconds={job.seconds}; resolution={job.resolution or 'server-default'}; "
        f"aspect_ratio={job.aspect_ratio or 'server-default'}; "
        f"image_refs={len(job.image_urls) + sum(r['type'] == 'image' for r in job.references)}; "
        f"video_refs={len(job.video_urls) + sum(r['type'] == 'video' for r in job.references)}; "
        f"audio_refs={len(job.audio_urls) + sum(r['type'] == 'audio' for r in job.references)}; "
        f"prompt_sha256={prompt_hash}"
    )
    emit("BILLING: 视频任务受理即可能产生费用；脚本不会自动重试或重复创建。")


def _get_path(payload, *paths, default=None):
    for path in paths:
        value = payload
        found = True
        for part in path.split("."):
            if not isinstance(value, dict) or part not in value:
                found = False
                break
            value = value[part]
        if found and value is not None:
            return value
    return default


def extract_task_id(payload):
    task_id = _get_path(
        payload,
        "id",
        "request_id",
        "task_id",
        "video.id",
        "video.request_id",
        "video.task_id",
        "data.id",
        "data.request_id",
        "data.task_id",
        "data.video.id",
        "data.video.request_id",
        "data.video.task_id",
    )
    task_id = str(task_id or "").strip()
    if not task_id:
        raise VideoError("视频接口响应没有任务 id；请保留原请求和响应。")
    return task_id


def extract_status(payload):
    status = _get_path(
        payload,
        "status",
        "state",
        "video.status",
        "data.status",
        "data.state",
        "data.video.status",
    )
    return str(status or "").strip().lower()


def extract_error(payload):
    value = _get_path(
        payload,
        "error.message",
        "error",
        "data.error.message",
        "data.error",
        "message",
        "detail",
    )
    if isinstance(value, dict):
        value = value.get("message") or json.dumps(value, ensure_ascii=False)
    return redact_secret(value or "视频任务失败，接口未提供错误信息")


def create_video_task(job, key, *, request_json_fn=None):
    request_json_fn = request_json_fn or request_json
    payload = build_video_payload(job)
    body = json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode(
        "utf-8"
    )
    try:
        result = request_json_fn(
            "/videos",
            key,
            payload=body,
            content_type="application/json; charset=utf-8",
            timeout=job.timeout,
        )
    except MediaError as exc:
        raise _with_platform_hint(exc) from None
    task_id = extract_task_id(result)
    result = dict(result)
    result["id"] = task_id
    return result


def video_task_path(task_id, suffix=""):
    escaped = urllib.parse.quote(str(task_id), safe="")
    if not escaped:
        raise VideoError("任务 id 不能为空。")
    return f"/videos/{escaped}{suffix}"


def get_video_task(task_id, key, timeout=120, *, request_json_fn=None):
    request_json_fn = request_json_fn or request_json
    return request_json_fn(
        video_task_path(task_id),
        key,
        timeout=timeout,
    )


def wait_for_video(
    task_id,
    key,
    *,
    poll_interval=5,
    poll_timeout=1800,
    sleeper=None,
    getter=None,
):
    sleeper = sleeper or time.sleep
    getter = getter or get_video_task
    history = []
    deadline = time.monotonic() + poll_timeout
    while True:
        result = getter(task_id, key, 120)
        if not isinstance(result, dict):
            raise VideoError(f"任务 {task_id} 查询响应不是 JSON 对象。")
        status = extract_status(result)
        history.append(result)
        if status in TERMINAL_SUCCESS:
            return result, history
        if status in TERMINAL_FAILURE:
            raise VideoTaskFailed(
                f"任务 {task_id} 状态为 {status}: {extract_error(result)}"
            )
        if status not in PENDING_STATUSES:
            raise VideoError(
                f"任务 {task_id} 返回未知状态 {status or 'empty'}；停止自动处理。"
            )
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise VideoTaskTimeout(
                f"任务 {task_id} 查询超时；不要重新创建，请继续查询原任务。"
            )
        sleeper(min(poll_interval, remaining))


def download_video(task_id, key, destination, *, timeout=900):
    if not str(task_id or "").strip():
        raise VideoError("任务 id 不能为空。")
    record = download_file(
        BASE_URL + video_task_path(task_id, "/content"),
        destination,
        headers={"Authorization": f"Bearer {key}"},
        timeout=timeout,
    )
    return record


def verify_video_available(model, key, *, request_json_fn=None):
    request_json_fn = request_json_fn or request_json
    result = request_json_fn("/models", key, timeout=120)
    items = result.get("data") if isinstance(result, dict) else None
    if not isinstance(items, list):
        raise VideoError("模型列表响应缺少 data 数组。")
    ids = {
        str(item.get("id"))
        for item in items
        if isinstance(item, dict) and item.get("id")
    }
    visible = model in ids
    return {
        "model": model,
        "visible": visible,
        "model_count": len(ids),
        "models": sorted(ids),
        "warning": (
            None
            if visible
            else "模型未出现在本次 /models 响应中。"
        ),
    }


def _with_platform_hint(exc):
    """把网关的平台/模型拒绝翻译成可操作的换 Key 或换模型提示。"""
    text = str(exc)
    lowered = text.lower()
    if "not supported for this platform" in lowered:
        return VideoError(
            text
            + "\n提示：当前 Key 的分组平台不提供 /v1/videos。"
            "/v1/videos 只接受 video / composite / grok 分组，生图组（openai）会被直接拒绝。\n"
            "处理：换用 video 分组的 Key；或换用该分组支持的视频模型。"
        )
    if "http 404" in lowered:
        return VideoError(
            text
            + "\n提示：404 常见于 Key 分组平台不匹配，或该模型在当前分组不可用。"
            "请核对 Key 是否为 video 分组，并确认模型名出现在 /models 列表中。"
        )
    return exc


VIDEO_KEY_HELP = (
    "未配置视频专用凭据 SSHZYU_VIDEO_API_KEY；不会回退到通用 SSHZYU_API_KEY。\n"
    "原因：视频与图片分属不同平台分组——/v1/videos 只接受 video / composite / grok 分组，"
    "生图组（openai）的 Key 必然被拒并返回 404。\n"
    "处理：\n"
    "  1) 到 https://sshzyu.com 申请一个 video 分组的 API Key；\n"
    "  2) 复制 sshzyu-video/.env.example 为 sshzyu-video/.env，填写 SSHZYU_VIDEO_API_KEY=...；\n"
    "  3) 或设置同名环境变量 SSHZYU_VIDEO_API_KEY。\n"
    "已配置仍失败时，请核对 Key 所属分组是否为 video，或换用其它视频模型。"
)


def require_video_key(env_file):
    """取视频专用 Key；缺失即失败，绝不回退到生图 Key。"""
    key, source, _generic = load_video_key(env_file)
    if not key:
        raise VideoError(VIDEO_KEY_HELP)
    emit(f"CREDENTIAL: {source}; no automatic retry")
    return key


def require_model_visible(model, key):
    """模型不在 /models 中即停止提交，避免提交后才 404 或空跑计费。"""
    visibility = verify_video_available(model, key)
    if visibility.get("visible"):
        return visibility
    available = visibility.get("models") or []
    raise VideoError(
        f"模型 {model} 未出现在 /models 响应中，已停止提交，未产生费用。\n"
        f"当前 Key 可见模型：{', '.join(available) if available else '未获取到'}。\n"
        "处理：\n"
        "  - 换用上面列出的视频模型（--model）；\n"
        "  - 或换用另一个 video 分组的 Key（不同分组的模型可见范围不同）；\n"
        "  - 确认模型确实可用时，加 --no-verify-model 跳过本检查。"
    )


def _task_directory(out_dir, job):
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
    name = f"video-{stamp}-{safe_filename(job.id)}-{uuid.uuid4().hex[:8]}"
    directory = Path(out_dir).expanduser().resolve() / name
    directory.mkdir(parents=True, exist_ok=False)
    return directory


def _video_result_record(
    job,
    status,
    directory,
    *,
    task_id=None,
    task=None,
    video=None,
    warnings=None,
    error=None,
    usage=None,
):
    record = {
        "id": job.id,
        "status": status,
        "endpoint": BASE_URL + job.endpoint,
        "model": job.model,
        "prompt_sha256": hashlib.sha256(job.prompt.encode("utf-8")).hexdigest(),
        "seconds_requested": job.seconds,
        "resolution_requested": job.resolution,
        "aspect_ratio_requested": job.aspect_ratio,
        "task_id": task_id,
        "task": task,
        "usage": usage,
        "video": video,
        "warnings": warnings or [],
        "error": error,
        "output_dir": str(directory),
    }
    write_json(Path(directory) / "job-result.json", record)
    return record


def execute_video_job(
    job,
    key,
    out_dir,
    *,
    create_fn=None,
    wait_fn=None,
    download_fn=None,
    probe_fn=None,
    verify_model=True,
):
    create_fn = create_fn or create_video_task
    wait_fn = wait_fn or wait_for_video
    download_fn = download_fn or download_video
    probe_fn = probe_fn or probe_media
    validate_video_job(job)
    directory = _task_directory(out_dir, job)
    payload = build_video_payload(job)
    write_json(directory / "request.json", payload)
    task_id = None
    task = None
    try:
        if verify_model:
            require_model_visible(job.model, key)
        submit = create_fn(job, key)
        task_id = extract_task_id(submit)
        write_json(directory / "task-submitted.json", submit)
        emit(f"[{job.id}] TASK: {task_id}; state=submitted")
        final, history = wait_fn(
            task_id,
            key,
            poll_interval=job.poll_interval,
            poll_timeout=job.poll_timeout,
        )
        task = final
        write_json(directory / "task-final.json", final)
        write_json(directory / "task-history.json", {"items": history})
        video_path = directory / "video.mp4"
        video = download_fn(task_id, key, video_path)
        container = video.get("container")
        if container and container != "mp4":
            suffix = ".webm" if container == "webm" else ".ts"
            renamed = directory / f"video{suffix}"
            Path(video["path"]).replace(renamed)
            video["path"] = str(renamed)
        warnings = []
        media_info = None
        if media_tools_status()["available"]:
            media_info = probe_fn(video["path"])
            video["media"] = media_info
            if media_info["duration"] <= 0:
                warnings.append("ffprobe 未读取到有效视频时长。")
            if (
                media_info.get("width")
                and media_info.get("height")
                and job.aspect_ratio not in (None, "auto", "adaptive")
            ):
                actual_ratio = media_info["width"] / media_info["height"]
                target_width, target_height = (
                    int(value)
                    for value in job.aspect_ratio.split(":")
                )
                target_ratio = target_width / target_height
                if abs(actual_ratio - target_ratio) / target_ratio > 0.03:
                    warnings.append("实际画幅与请求画幅差异超过 3%。")
        else:
            warnings.append("未安装 ffprobe，未校验视频时长和画幅。")
        status = "partial" if warnings else "success"
        usage = _get_path(task, "usage", "data.usage")
        record = _video_result_record(
            job,
            status,
            directory,
            task_id=task_id,
            task=task,
            video=video,
            warnings=warnings,
            usage=usage,
        )
        emit(f"[{job.id}] VIDEO: {video['path']}")
        emit(f"[{job.id}] STATUS: {status}")
        return record
    except (MediaError, OSError, UnicodeError) as exc:
        error = redact_secret(exc, key)
        record = _video_result_record(
            job,
            "error",
            directory,
            task_id=task_id,
            task=task,
            error=error,
        )
        emit(f"[{job.id}] ERROR: {error}", error=True)
        if task_id:
            emit(
                f"[{job.id}] KEEP TASK: {task_id}; "
                "不要重新创建，后续使用 wait 查询原任务。",
                error=True,
            )
        return record


def _add_job_arguments(parser):
    parser.add_argument("prompt", nargs="?", help="视频提示词")
    parser.add_argument("--prompt", dest="prompt_option", help="视频提示词")
    parser.add_argument("--prompt-file", help="UTF-8 提示词文件")
    parser.add_argument("--id", default="video-01", help="任务友好名称")
    parser.add_argument("--model", required=True, help="当前 Key 可用的精确视频模型名")
    parser.add_argument("--seconds", type=int, required=True, help="生成秒数")
    parser.add_argument("--resolution", help="480p/720p/768p/1080p/2k/4k")
    parser.add_argument("--aspect-ratio", help="16:9、9:16、1:1、4:3、3:4、21:9")
    parser.add_argument("--image-url", action="append", default=[], help="公网 HTTPS 参考图，可重复")
    parser.add_argument("--video-url", action="append", default=[], help="公网 HTTPS 参考视频，可重复")
    parser.add_argument("--audio-url", action="append", default=[], help="公网 HTTPS 参考音频，可重复")
    parser.add_argument("--references-file", help="references 对象数组 JSON 文件")
    parser.add_argument("--timeout", type=float, default=600, help="提交请求超时秒数")
    parser.add_argument("--poll-interval", type=float, default=5)
    parser.add_argument("--poll-timeout", type=float, default=1800)
    parser.add_argument("--out", default="./sshzyu-video")
    parser.add_argument("--env-file", default=str(VIDEO_ENV_FILE))
    parser.add_argument("--dry-run", action="store_true", help="只校验计划，不读 Key、不联网")
    parser.add_argument("--confirm", action="store_true", help="用户已明确确认视频计划")
    parser.add_argument(
        "--no-verify-model",
        action="store_true",
        help="不调用免费的 /models 做提交前权限核对",
    )


def _job_from_args(args):
    prompt = args.prompt_option or args.prompt or ""
    if args.prompt_file:
        if prompt:
            raise VideoError("prompt、位置参数和 --prompt-file 只能使用一种。")
        prompt_path = Path(args.prompt_file).expanduser().resolve()
        if not prompt_path.is_file():
            raise VideoError(f"提示词文件不存在: {prompt_path}")
        prompt = prompt_path.read_text(encoding="utf-8-sig")
    references = []
    if args.references_file:
        if args.image_url or args.video_url or args.audio_url:
            raise VideoError("--references-file 与 --image-url/--video-url/--audio-url 不能混用。")
        path = Path(args.references_file).expanduser().resolve()
        try:
            references = json.loads(path.read_text(encoding="utf-8-sig"))
        except FileNotFoundError:
            raise VideoError(f"references 文件不存在: {path}") from None
        except (ValueError, UnicodeError) as exc:
            raise VideoError(f"references JSON 无效: {redact_secret(exc)}") from None
    job = VideoJob(
        id=args.id,
        prompt=prompt,
        model=args.model,
        seconds=args.seconds,
        resolution=args.resolution,
        aspect_ratio=args.aspect_ratio,
        image_urls=args.image_url,
        video_urls=args.video_url,
        audio_urls=args.audio_url,
        references=references,
        timeout=args.timeout,
        poll_interval=args.poll_interval,
        poll_timeout=args.poll_timeout,
    )
    return validate_video_job(job)


def parser():
    p = argparse.ArgumentParser(
        prog="sshzyu_video.py",
        description="SSHZYU 异步视频任务：创建、查询、等待和下载。",
    )
    sub = p.add_subparsers(dest="command", required=True)
    run = sub.add_parser("run", help="创建任务、等待并下载")
    _add_job_arguments(run)
    create = sub.add_parser("create", help="只创建任务并保存任务 id")
    _add_job_arguments(create)
    get = sub.add_parser("get", help="查询任务状态")
    get.add_argument("task_id")
    get.add_argument("--env-file", default=str(VIDEO_ENV_FILE))
    wait = sub.add_parser("wait", help="等待现有任务")
    wait.add_argument("task_id")
    wait.add_argument("--poll-interval", type=float, default=5)
    wait.add_argument("--poll-timeout", type=float, default=1800)
    wait.add_argument("--download", action="store_true")
    wait.add_argument("--out", default="./sshzyu-video")
    wait.add_argument("--env-file", default=str(VIDEO_ENV_FILE))
    download = sub.add_parser("download", help="下载已完成任务")
    download.add_argument("task_id")
    download.add_argument("--out", default="./sshzyu-video")
    download.add_argument("--env-file", default=str(VIDEO_ENV_FILE))
    return p


def _download_destination(out, task_id):
    target = Path(out).expanduser().resolve()
    if target.suffix.lower() == ".mp4" or not target.exists():
        if target.suffix and target.suffix.lower() not in (".mp4", ".webm", ".mkv", ".mov"):
            raise VideoError("下载输出必须是目录或视频文件路径。")
        if target.suffix:
            return target
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
    target.mkdir(parents=True, exist_ok=True)
    return target / f"{safe_filename(task_id)}-{stamp}.mp4"


def main(argv=None):
    args = parser().parse_args(argv)
    key = None
    try:
        if args.command in ("run", "create"):
            job = _job_from_args(args)
            print_video_plan(job)
            if args.dry_run:
                return 0
            if not args.confirm:
                raise VideoError(
                    "真实视频任务会产生费用；完成 preflight 并取得用户确认后加 --confirm。"
                )
            key = require_video_key(args.env_file)
            if args.command == "run":
                result = execute_video_job(
                    job,
                    key,
                    args.out,
                    verify_model=not args.no_verify_model,
                )
                return 0 if result["status"] == "success" else 2 if result["status"] == "partial" else 1
            if not args.no_verify_model:
                require_model_visible(job.model, key)
            directory = _task_directory(args.out, job)
            write_json(directory / "request.json", build_video_payload(job))
            submit = create_video_task(job, key)
            task_id = extract_task_id(submit)
            write_json(directory / "task-submitted.json", submit)
            write_json(
                directory / "job-result.json",
                {
                    "id": job.id,
                    "status": "submitted",
                    "task_id": task_id,
                    "task": submit,
                    "output_dir": str(directory),
                },
            )
            emit(f"TASK: {task_id}")
            emit(f"RECORD: {directory / 'job-result.json'}")
            return 0
        key = require_video_key(args.env_file)
        if args.command == "get":
            result = get_video_task(args.task_id, key)
            emit(json.dumps(result, ensure_ascii=False, indent=2))
            return 0
        if args.command == "wait":
            final, history = wait_for_video(
                args.task_id,
                key,
                poll_interval=args.poll_interval,
                poll_timeout=args.poll_timeout,
            )
            emit(json.dumps(final, ensure_ascii=False, indent=2))
            if args.download:
                destination = _download_destination(args.out, args.task_id)
                record = download_video(args.task_id, key, destination)
                emit(json.dumps(record, ensure_ascii=False, indent=2))
            return 0
        if args.command == "download":
            destination = _download_destination(args.out, args.task_id)
            record = download_video(args.task_id, key, destination)
            emit(json.dumps(record, ensure_ascii=False, indent=2))
            return 0
        raise VideoError(f"未知命令: {args.command}")
    except (MediaError, OSError, UnicodeError) as exc:
        emit("ERROR: " + redact_secret(exc, key), error=True)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
