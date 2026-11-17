#!/usr/bin/env python3
"""Unified SSHZYU media skill CLI for images, videos, and short-drama workflows."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
video_scripts = Path(__file__).resolve().parents[2] / "sshzyu-video" / "scripts"
if not video_scripts.is_dir():
    import os
    candidates = [Path(os.environ["NEWMAX_SKILLS_DIR"]) / "sshzyu-video" / "scripts"] if os.environ.get("NEWMAX_SKILLS_DIR") else []
    candidates += [Path.home() / ".newmax" / "skills" / "sshzyu-video" / "scripts", Path.home() / ".codex" / "skills" / "sshzyu-video" / "scripts"]
    video_scripts = next((candidate for candidate in candidates if (candidate / "sshzyu_video.py").is_file()), video_scripts)
if video_scripts.is_dir():
    sys.path.insert(1, str(video_scripts))

from media_common import (
    MediaError,
    classify_model,
    media_tools_status,
    redact_secret,
)
from sshzyu_drama import main as drama_main
from sshzyu_image import (
    DEFAULT_ENV_FILE,
    ENV_KEY,
    read_credential,
    read_env_key,
    request_json,
    load_key,
)
from sshzyu_image import main as image_main
from sshzyu_video import main as video_main


VERSION = "2.0.0"
COMMANDS = {"image", "video", "drama", "models", "doctor"}


def emit(message, error=False):
    print(message, file=sys.stderr if error else sys.stdout, flush=True)


def help_text():
    return """SSHZYU Media Skill

用法:
  sshzyu_media.py image [旧版生图参数...]
  sshzyu_media.py video run|create|get|wait|download ...
  sshzyu_media.py drama run|assemble ...
  sshzyu_media.py models [--kind image|video|other|all]
  sshzyu_media.py doctor

兼容:
  直接传旧版生图参数时，自动路由到 image。

安全边界:
  真实生图和视频提交需要 --confirm；dry-run 不读取 Key、不联网。
  视频参考素材只接受公网 HTTPS 直链，不接受本地路径或内网 URL。
"""


def filter_models(payload, kind="all"):
    if kind not in {"all", "image", "video", "other"}:
        raise MediaError("--kind 必须是 all、image、video 或 other。")
    if not isinstance(payload, dict) or not isinstance(payload.get("data"), list):
        raise MediaError("模型列表响应缺少 data 数组。")
    items = []
    for item in payload["data"]:
        if not isinstance(item, dict) or not item.get("id"):
            continue
        row = dict(item)
        row["kind"] = classify_model(row["id"])
        if kind == "all" or row["kind"] == kind:
            items.append(row)
    return {
        "object": payload.get("object", "list"),
        "data": items,
        "filtered_count": len(items),
    }


def models_main(argv):
    parser = argparse.ArgumentParser(
        prog="sshzyu_media.py models",
        description="读取当前 Key 可见模型并按媒体类型过滤。"
    )
    parser.add_argument("--kind", choices=["all", "image", "video", "other"], default="all")
    parser.add_argument("--env-file", default=str(DEFAULT_ENV_FILE))
    parser.add_argument("--timeout", type=float, default=120)
    parser.add_argument("--raw", action="store_true", help="输出接口原始模型列表")
    args = parser.parse_args(argv)
    key = None
    try:
        key, source = load_key(args.env_file)
        result = request_json("/models", key, timeout=args.timeout)
        output = result if args.raw else filter_models(result, args.kind)
        emit(json.dumps(output, ensure_ascii=False, indent=2))
        emit(f"CREDENTIAL: {source}")
        return 0
    except (MediaError, OSError, UnicodeError) as exc:
        emit("ERROR: " + redact_secret(exc, key), error=True)
        return 1


def doctor_main(argv):
    parser = argparse.ArgumentParser(
        prog="sshzyu_media.py doctor",
        description="检查运行时、凭据来源和媒体工具，不联网。"
    )
    parser.add_argument("--env-file", default=str(DEFAULT_ENV_FILE))
    args = parser.parse_args(argv)
    try:
        path = Path(args.env_file).expanduser().resolve()
        env_key = read_env_key(path)
        process_key = bool(os.environ.get(ENV_KEY))
        credential_key = False
        credential_error = None
        try:
            credential_key = bool(read_credential())
        except MediaError as exc:
            credential_error = redact_secret(exc)
        source = (
            f"env:{path}"
            if env_key
            else "process environment"
            if process_key
            else "Windows Credential Manager fallback"
            if credential_key
            else "not-loaded"
        )
        result = {
            "version": VERSION,
            "python": sys.version.split()[0],
            "skill_root": str(Path(__file__).resolve().parent.parent),
            "env_file": str(path),
            "env_file_exists": path.exists(),
            "key_present": bool(env_key or process_key or credential_key),
            "credential_source": source,
            "credential_error": credential_error,
            "endpoint": "https://sshzyu.com/v1",
            "network_tested": False,
            "media_tools": media_tools_status(),
        }
        emit(json.dumps(result, ensure_ascii=False, indent=2))
        return 0
    except (MediaError, OSError, UnicodeError) as exc:
        emit("ERROR: " + redact_secret(exc), error=True)
        return 1


def main(argv=None):
    argv = list(sys.argv[1:] if argv is None else argv)
    if not argv:
        emit(help_text(), error=True)
        return 2
    if argv[0] in ("-h", "--help", "help"):
        emit(help_text())
        return 0
    if argv[0] in ("--version", "version"):
        emit(VERSION)
        return 0
    command = argv[0]
    rest = argv[1:]
    try:
        if command == "image":
            return image_main(rest)
        if command == "video":
            return video_main(rest)
        if command == "drama":
            return drama_main(rest)
        if command == "models":
            return models_main(rest)
        if command == "doctor":
            return doctor_main(rest)
        return image_main(argv)
    except (MediaError, OSError, UnicodeError) as exc:
        emit("ERROR: " + redact_secret(exc), error=True)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
