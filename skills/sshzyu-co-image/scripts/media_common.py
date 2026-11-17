#!/usr/bin/env python3
"""Shared security, validation, and media helpers for the SSHZYU media skill."""

from __future__ import annotations

import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import urllib.error
import urllib.parse
import urllib.request
import uuid


MAX_DOWNLOAD = 2 * 1024 * 1024 * 1024
CHUNK_SIZE = 1024 * 1024
MAGIC_SAMPLE = 64
RESOLUTION_RE = re.compile(r"^(?:auto|adaptive|[1-9]\d{2,3}p|[124]k)$")
MODEL_RE = re.compile(r"^[^\s\x00-\x1f]{1,200}$")
SHOT_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$")
VIDEO_PREFIXES = (
    "sd-",
    "sd2",
    "sd25",
    "seedance",
    "doubao-seedance",
    "minimax_h3",
    "videopipe-",
)


class MediaError(Exception):
    """A safe, user-facing media error."""


def redact_secret(value, secret=None):
    text = str(value)
    if secret:
        text = text.replace(secret, "[REDACTED]")
    text = re.sub(r"(?<![A-Za-z0-9])sk-[A-Za-z0-9_-]+", "[REDACTED]", text)
    text = re.sub(r"(?i)Bearer\s+\S+", "Bearer [REDACTED]", text)
    text = re.sub(
        r"(?i)([?&](?:sig|signature|token|key|expires|policy)=)[^&\s]+",
        r"\1[REDACTED]",
        text,
    )
    return text[:2000]


def classify_model(model_id):
    model = str(model_id or "").strip()
    if model.startswith("gpt-image-"):
        return "image"
    if model.startswith(VIDEO_PREFIXES):
        return "video"
    return "other"


def safe_filename(value, fallback="job"):
    text = str(value or "").strip()
    text = text.replace("\\", "/").replace("/", "_")
    text = re.sub(r"[^A-Za-z0-9._-]+", "-", text).strip("-._")
    return (text or fallback)[:80]


def validate_model_id(value):
    model = str(value or "").strip()
    if not model or not MODEL_RE.fullmatch(model):
        raise MediaError("模型名称为空或包含空白、控制字符。")
    return model


def validate_shot_id(value):
    shot_id = str(value or "").strip()
    if not SHOT_ID_RE.fullmatch(shot_id):
        raise MediaError(
            "镜头 id 只能包含字母、数字、点、下划线和短横线，最长 80 字符。"
        )
    return shot_id


def validate_resolution(value):
    if value is None or value == "":
        return None
    normalized = str(value).strip().lower()
    if not RESOLUTION_RE.fullmatch(normalized):
        raise MediaError(
            "resolution 必须是 480p、720p、768p、1080p、2k、4k、auto 或 adaptive。"
        )
    return normalized


def validate_aspect_ratio(value):
    if value is None or value == "":
        return None
    normalized = str(value).strip().lower()
    allowed = {
        "auto",
        "adaptive",
        "16:9",
        "9:16",
        "1:1",
        "4:3",
        "3:4",
        "21:9",
        "9:21",
    }
    if normalized not in allowed:
        raise MediaError("aspect_ratio 不在 SSHZYU 视频接口支持的画幅列表内。")
    return normalized


def _is_public_hostname(hostname):
    host = (hostname or "").strip().rstrip(".").lower()
    if not host or host == "localhost" or host.endswith(".localhost") or host.endswith(".local"):
        return False
    try:
        address = ipaddress.ip_address(host)
    except ValueError:
        return True
    return address.is_global


def validate_reference_url(value, field="reference URL"):
    url = str(value or "").strip()
    parsed = urllib.parse.urlsplit(url)
    if parsed.scheme.lower() != "https":
        raise MediaError(f"{field} 必须是公网 HTTPS 直链。")
    if parsed.username is not None or parsed.password is not None:
        raise MediaError(f"{field} 不能包含用户名或密码。")
    if not parsed.hostname or not _is_public_hostname(parsed.hostname):
        raise MediaError(f"{field} 不能指向 localhost、内网或非公网地址。")
    if parsed.fragment:
        raise MediaError(f"{field} 不能包含 URL fragment。")
    return url


def detect_video_container(data):
    if not isinstance(data, (bytes, bytearray)):
        return None
    payload = bytes(data[:MAGIC_SAMPLE])
    if len(payload) >= 12 and payload[4:8] == b"ftyp":
        return "mp4"
    if payload.startswith(b"\x1aE\xdf\xa3"):
        return "webm"
    if payload and payload[0] == 0x47:
        return "mpegts"
    return None


def is_video_payload(data):
    return detect_video_container(data) is not None


def _origin(url):
    parsed = urllib.parse.urlsplit(url)
    port = parsed.port
    if port is None:
        port = 443 if parsed.scheme.lower() == "https" else 80
    return parsed.scheme.lower(), (parsed.hostname or "").lower(), port


class SafeRedirectHandler(urllib.request.HTTPRedirectHandler):
    """Allow media redirects while preventing credentials crossing origins."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        target = urllib.parse.urljoin(req.full_url, newurl)
        parsed = urllib.parse.urlsplit(target)
        if parsed.scheme.lower() not in ("http", "https"):
            raise MediaError("下载重定向只允许 HTTP/HTTPS 地址。")
        old_parsed = urllib.parse.urlsplit(req.full_url)
        if old_parsed.scheme.lower() == "https" and parsed.scheme.lower() != "https":
            raise MediaError("下载重定向不允许从 HTTPS 降级到 HTTP。")
        if _is_public_hostname(old_parsed.hostname) and not _is_public_hostname(
            parsed.hostname
        ):
            raise MediaError("下载重定向不能从公网地址跳到 localhost、内网或非公网地址。")
        same_origin = _origin(req.full_url) == _origin(target)
        old_headers = {key.lower(): value for key, value in req.header_items()}
        if same_origin:
            safe_headers = old_headers
        else:
            safe_headers = {
                key: value
                for key, value in old_headers.items()
                if key in {"accept", "user-agent"}
            }
        return urllib.request.Request(target, headers=safe_headers, method="GET")


def download_file(
    url,
    destination,
    *,
    headers=None,
    timeout=900,
    max_bytes=MAX_DOWNLOAD,
):
    path = Path(destination).expanduser().resolve()
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        raise MediaError(f"下载目标已存在，不覆盖: {path}")
    part = path.with_name(path.name + f".part-{uuid.uuid4().hex}")
    request_headers = {
        "Accept": "video/*,application/octet-stream;q=0.9,*/*;q=0.1",
        "Accept-Encoding": "identity",
        "User-Agent": "sshzyu-media-skill/2.0",
        **(headers or {}),
    }
    opener = urllib.request.build_opener(SafeRedirectHandler())
    digest = hashlib.sha256()
    total = 0
    sample = b""
    content_type = ""
    try:
        request = urllib.request.Request(url, headers=request_headers, method="GET")
        with opener.open(request, timeout=timeout) as response:
            status = getattr(response, "status", 200)
            content_type = response.headers.get("Content-Type", "")
            content_length = response.headers.get("Content-Length")
            if status == 206:
                content_range = response.headers.get("Content-Range", "")
                match = re.fullmatch(r"bytes\s+(\d+)-(\d+)/(\d+|\*)", content_range)
                if not match:
                    raise MediaError("分段下载缺少有效 Content-Range。")
                start, end, total_size = match.groups()
                if int(start) != 0 or (
                    total_size != "*" and int(end) + 1 != int(total_size)
                ):
                    raise MediaError("服务端返回了不完整的分段内容。")
            if content_length:
                try:
                    if int(content_length) > max_bytes:
                        raise MediaError("下载内容超过本地大小限制。")
                except ValueError:
                    pass
            with part.open("xb") as stream:
                while True:
                    chunk = response.read(CHUNK_SIZE)
                    if not chunk:
                        break
                    total += len(chunk)
                    if total > max_bytes:
                        raise MediaError("下载内容超过本地大小限制。")
                    if len(sample) < MAGIC_SAMPLE:
                        sample += chunk[: MAGIC_SAMPLE - len(sample)]
                    digest.update(chunk)
                    stream.write(chunk)
        if total <= 0:
            raise MediaError("下载响应为空。")
        container = detect_video_container(sample)
        if container is None:
            raise MediaError("下载内容不是可识别的视频文件；拒绝保存。")
        os.replace(part, path)
    except MediaError:
        raise
    except urllib.error.HTTPError as exc:
        raise MediaError(f"视频下载失败: HTTP {exc.code}。") from None
    except (urllib.error.URLError, TimeoutError, OSError) as exc:
        raise MediaError(f"视频下载网络失败: {redact_secret(exc)}") from None
    finally:
        if part.exists():
            part.unlink()
    return {
        "path": str(path),
        "bytes": total,
        "sha256": digest.hexdigest(),
        "content_type": content_type,
        "kind": "video",
        "container": container,
    }


def write_json(path, data):
    target = Path(path).expanduser().resolve()
    target.parent.mkdir(parents=True, exist_ok=True)
    temp = target.with_name(target.name + f".tmp-{uuid.uuid4().hex}")
    try:
        temp.write_text(
            json.dumps(data, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
        os.replace(temp, target)
    finally:
        if temp.exists():
            temp.unlink()
    return target


def read_json(path):
    source = Path(path).expanduser().resolve()
    try:
        return json.loads(source.read_text(encoding="utf-8-sig"))
    except FileNotFoundError:
        raise MediaError(f"文件不存在: {source}") from None
    except (ValueError, UnicodeError) as exc:
        raise MediaError(f"JSON 无效: {redact_secret(exc)}") from None


def _tool_path(name):
    return shutil.which(name)


def media_tools_status():
    ffmpeg = _tool_path("ffmpeg")
    ffprobe = _tool_path("ffprobe")
    return {
        "ffmpeg": ffmpeg,
        "ffprobe": ffprobe,
        "available": bool(ffmpeg and ffprobe),
    }


def probe_media(path, *, timeout=30):
    source = Path(path).expanduser().resolve()
    if not source.is_file():
        raise MediaError(f"媒体文件不存在: {source}")
    ffprobe = _tool_path("ffprobe")
    if not ffprobe:
        raise MediaError("未找到 ffprobe，无法校验视频。")
    command = [
        ffprobe,
        "-v",
        "error",
        "-show_entries",
        "format=duration,format_name,size:stream=index,codec_type,codec_name,width,height,r_frame_rate",
        "-of",
        "json",
        str(source),
    ]
    result = subprocess.run(
        command,
        check=False,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        timeout=timeout,
    )
    if result.returncode != 0:
        raise MediaError(f"ffprobe 校验失败: {redact_secret(result.stderr.strip())}")
    try:
        payload = json.loads(result.stdout)
    except ValueError:
        raise MediaError("ffprobe 未返回有效 JSON。") from None
    streams = payload.get("streams") or []
    video_stream = next(
        (stream for stream in streams if stream.get("codec_type") == "video"),
        None,
    )
    if not video_stream:
        raise MediaError("文件中没有视频流。")
    format_info = payload.get("format") or {}
    try:
        duration = float(format_info.get("duration") or 0)
    except (TypeError, ValueError):
        duration = 0.0
    return {
        "duration": duration,
        "format_name": format_info.get("format_name"),
        "size": int(format_info.get("size") or source.stat().st_size),
        "width": int(video_stream.get("width") or 0),
        "height": int(video_stream.get("height") or 0),
        "video_codec": video_stream.get("codec_name"),
        "frame_rate": video_stream.get("r_frame_rate"),
        "has_audio": any(stream.get("codec_type") == "audio" for stream in streams),
    }


def run_checked(command, *, timeout=7200):
    result = subprocess.run(
        command,
        check=False,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        timeout=timeout,
    )
    if result.returncode != 0:
        message = (result.stderr or result.stdout or "").strip()
        raise MediaError(f"命令执行失败: {redact_secret(message)}")
    return result


def make_temp_file(parent, suffix):
    directory = Path(parent).expanduser().resolve()
    directory.mkdir(parents=True, exist_ok=True)
    handle = tempfile.NamedTemporaryFile(
        prefix=".sshzyu-",
        suffix=suffix,
        dir=directory,
        delete=False,
    )
    handle.close()
    return Path(handle.name)
