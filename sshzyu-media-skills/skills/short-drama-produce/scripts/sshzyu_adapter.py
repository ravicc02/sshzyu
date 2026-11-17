#!/usr/bin/env python3
"""SSHZYU 视频适配器：按 short-drama-produce 的 adapter 协议提交视频任务。

从 stdin 读取任务单与运行上下文，向 stdout 输出 outputs 或白名单错误。
只处理 video；图片走独立的 sshzyu-image。视频与图片分属不同平台分组，
因此这里要求视频专用凭据 SSHZYU_VIDEO_API_KEY，不接受通用生图 Key 回退。
"""

from __future__ import annotations

import argparse
import json
import shutil
import sys
from pathlib import Path

MAX_DETAIL = 160
PROVIDER = "sshzyu-video"


def _emit_error(category, code, detail=None, *, retryable=False, submitted=False):
    payload = {
        "error": {
            "provider": PROVIDER,
            "category": category,
            "code": code,
            "http_status": None,
            "request_id": None,
            "retryable": bool(retryable),
            "submitted": bool(submitted),
        }
    }
    if detail:
        payload["error"]["detail"] = str(detail)[:MAX_DETAIL]
    print(json.dumps(payload, ensure_ascii=False))
    return 1


def _suite_root():
    return Path(__file__).resolve().parents[2]


def _video_module():
    scripts = _suite_root() / "sshzyu-video" / "scripts"
    if not (scripts / "sshzyu_video.py").is_file():
        return None
    if str(scripts) not in sys.path:
        sys.path.insert(0, str(scripts))
    import sshzyu_video

    return sshzyu_video


def _write_handle(handle_path, task_id):
    if not handle_path:
        return
    target = Path(handle_path)
    target.parent.mkdir(parents=True, exist_ok=True)
    tmp = target.with_name(target.name + ".tmp")
    tmp.write_text(
        json.dumps({"provider_job_id": task_id}, ensure_ascii=False),
        encoding="utf-8",
    )
    tmp.replace(target)


def _url_list(params, name):
    values = params.get(name)
    if values is None:
        return []
    if isinstance(values, str):
        values = [values]
    if not isinstance(values, list):
        return []
    return [str(item) for item in values if str(item).startswith("https://")]


def _prompt_of(request, params):
    for candidate in (request.get("prompt"), params.get("prompt")):
        if isinstance(candidate, str) and candidate.strip():
            return candidate
    return None


def _image_module():
    scripts = _suite_root() / "sshzyu-image" / "scripts"
    if not (scripts / "sshzyu_image.py").is_file():
        return None
    if str(scripts) not in sys.path:
        sys.path.insert(0, str(scripts))
    import sshzyu_image

    return sshzyu_image


def _run_image(request, params, args, outputs, output_root):
    import contextlib

    if request.get("reference_bindings"):
        return _emit_error(
            "invalid_request",
            "reference_not_supported",
            "图片适配器当前只做无参考文生图；带参考会走 /images/edits 编辑接口，请先用 sshzyu-image CLI 处理",
        )

    module = _image_module()
    if module is None:
        return _emit_error("configuration", "missing_image_skill", "未找到同根安装的 sshzyu-image")

    try:
        key, _source = module.load_key(module.DEFAULT_ENV_FILE)
    except Exception:
        return _emit_error(
            "configuration",
            "image_credential_missing",
            "请复制 sshzyu-image/.env.example 为 .env 并填写 SSHZYU_API_KEY",
        )

    prompt = _prompt_of(request, params)
    if not prompt:
        return _emit_error("invalid_request", "missing_prompt", "任务单缺少 prompt")

    model = str(params.get("model") or args.image_model or module.DEFAULT_MODEL).strip()
    size = str(params.get("size") or args.size or module.DEFAULT_SIZE).strip()
    quality = str(params.get("quality") or module.DEFAULT_QUALITY).strip()
    suffix = Path(str(outputs[0])).suffix.lstrip(".").lower() or module.DEFAULT_FORMAT
    output_format = "jpeg" if suffix in ("jpg", "jpeg") else suffix

    try:
        job = module.Job(
            id=str(request.get("job_id") or "job"),
            prompt=prompt,
            model=model,
            size=size,
            quality=quality,
            output_format=output_format,
            n=1,
        )
    except Exception as exc:
        return _emit_error("invalid_request", "invalid_job_parameter", str(exc))

    try:
        # execute_job 会向 stdout 打印进度；协议要求 stdout 只有最终 JSON。
        with contextlib.redirect_stdout(sys.stderr):
            result = module.execute_job(0, job, key, Path(output_root))
    except Exception as exc:
        return _emit_error(
            "provider_response",
            "image_request_failed",
            module.redact(str(exc), key),
            submitted=True,
        )

    status = str(getattr(result, "status", "") or "")
    if status not in ("success", "partial"):
        detail = getattr(result, "error", None) or "图片生成失败"
        return _emit_error(
            "provider_response",
            "image_task_failed",
            detail,
            submitted=True,
        )

    images = getattr(result, "images", []) or []
    if not images or not images[0].get("path"):
        return _emit_error("provider_response", "missing_output", "未得到图片文件", submitted=True)

    # 协议要求 source 直接位于 output_root；execute_job 会建 00-<slug>/ 子目录。
    source = Path(images[0]["path"])
    destination = Path(output_root) / f"{job.id}{source.suffix}"
    if source != destination:
        shutil.move(str(source), str(destination))

    print(
        json.dumps(
            {
                "outputs": [{"target": outputs[0], "source": str(destination)}],
            },
            ensure_ascii=False,
        )
    )
    return 0


def main(argv=None):
    # production_tool 按 UTF-8 读写适配器的 stdin/stdout；Windows 默认 cp936 会让
    # 含中文的错误 JSON 解析失败，只剩一个无信息的 adapter_exit_N。
    for stream in (sys.stdin, sys.stdout):
        if hasattr(stream, "reconfigure"):
            stream.reconfigure(encoding="utf-8")

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--model", default=None, help="默认视频模型；任务单 parameters.model 优先")
    parser.add_argument("--resolution", default=None, help="默认视频分辨率档位")
    parser.add_argument("--image-model", default=None, help="默认图片模型")
    parser.add_argument("--size", default=None, help="默认图片尺寸，如 1024x1536")
    parser.add_argument("--poll-interval", type=float, default=5.0)
    parser.add_argument("--poll-timeout", type=float, default=1800.0)
    args = parser.parse_args(argv)

    try:
        request = json.load(sys.stdin)
    except (json.JSONDecodeError, UnicodeDecodeError):
        return _emit_error("invalid_request", "invalid_stdin", "stdin 不是合法 JSON")
    if not isinstance(request, dict):
        return _emit_error("invalid_request", "invalid_stdin", "任务单必须是对象")

    modality = str(request.get("modality") or "").strip()
    if modality not in ("video", "image"):
        return _emit_error(
            "invalid_request",
            "unsupported_modality",
            "本适配器只处理 video 与 image",
        )

    outputs = request.get("outputs") or []
    if not isinstance(outputs, list) or len(outputs) != 1:
        return _emit_error("invalid_request", "invalid_outputs", "任务必须且只能有一个输出路径")

    output_root = request.get("output_root")
    if not output_root:
        return _emit_error("invalid_request", "missing_output_root", "缺少 output_root")

    params = request.get("parameters")
    if not isinstance(params, dict):
        params = {}

    if modality == "image":
        return _run_image(request, params, args, outputs, output_root)

    module = _video_module()
    if module is None:
        return _emit_error(
            "configuration",
            "missing_video_skill",
            "未找到同根安装的 sshzyu-video",
        )

    try:
        key, _source, _generic = module.load_video_key(None)
    except Exception:
        key = None
    if not key:
        return _emit_error(
            "configuration",
            "video_credential_missing",
            "未配置 SSHZYU_VIDEO_API_KEY；视频只接受 video 分组 Key，不回退生图 Key。"
            "请复制 sshzyu-video/.env.example 为 sshzyu-video/.env 并填写，"
            "Key 到 https://sshzyu.com 申请 video 分组。",
        )

    model = str(params.get("model") or args.model or "").strip()
    if not model:
        return _emit_error(
            "configuration",
            "missing_model",
            "任务单未提供 parameters.model，适配器也未配置 --model",
        )

    seconds = params.get("duration", params.get("seconds"))
    if seconds is None:
        return _emit_error("invalid_request", "missing_duration", "视频任务需要 parameters.duration")

    prompt = _prompt_of(request, params)
    if not prompt:
        return _emit_error("invalid_request", "missing_prompt", "任务单缺少 prompt")

    image_urls = _url_list(params, "image_urls")
    video_urls = _url_list(params, "video_urls")
    audio_urls = _url_list(params, "audio_urls")
    bindings = request.get("reference_bindings") or []
    if bindings and not (image_urls or video_urls or audio_urls):
        return _emit_error(
            "invalid_request",
            "local_reference_unsupported",
            "视频接口只接受公网 HTTPS 参考素材；本地参考需先上传或改用 parameters.image_urls",
        )

    try:
        job = module.VideoJob(
            id=str(request.get("job_id") or "job"),
            prompt=prompt,
            model=model,
            seconds=int(seconds),
            resolution=params.get("resolution") or args.resolution,
            aspect_ratio=params.get("ratio") or params.get("aspect_ratio"),
            image_urls=image_urls,
            video_urls=video_urls,
            audio_urls=audio_urls,
            poll_interval=args.poll_interval,
            poll_timeout=args.poll_timeout,
        )
        module.validate_video_job(job)
    except Exception as exc:
        return _emit_error("invalid_request", "invalid_job_parameter", str(exc))

    destination = Path(output_root) / f"{job.id}.mp4"
    destination.parent.mkdir(parents=True, exist_ok=True)

    collect_id = request.get("collect_provider_job_id")
    task_id = str(collect_id) if collect_id else None

    try:
        if not task_id:
            submit = module.create_video_task(job, key)
            task_id = module.extract_task_id(submit)
            _write_handle(request.get("handle_path"), task_id)
        # collect 与首次执行都要先等到终态；未完成时直接下载会得到 HTTP 400。
        final, _history = module.wait_for_video(
            task_id,
            key,
            poll_interval=args.poll_interval,
            poll_timeout=args.poll_timeout,
        )
        status = str(final.get("status") or "").lower()
        if status in getattr(module, "TERMINAL_FAILURE", set()):
            return _emit_error(
                "provider_response",
                "video_task_failed",
                f"远端任务状态 {status}",
                submitted=True,
            )
        record = module.download_video(task_id, key, destination)
        actual = Path(record.get("path") or destination)
        if not actual.is_file():
            return _emit_error(
                "provider_response",
                "missing_output",
                "未下载到视频文件",
                submitted=True,
            )
    except Exception as exc:
        return _emit_error(
            "provider_response",
            "video_request_failed",
            module.redact_secret(str(exc), key),
            submitted=bool(task_id),
        )

    print(
        json.dumps(
            {
                "outputs": [{"target": outputs[0], "source": str(actual)}],
                "provider_job_id": task_id,
            },
            ensure_ascii=False,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
