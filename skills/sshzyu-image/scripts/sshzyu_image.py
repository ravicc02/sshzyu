#!/usr/bin/env python3
"""SSHZYU batch image generation/editing CLI using only the Python standard library."""

import argparse
import base64
import binascii
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from datetime import datetime, timezone
import getpass
import hashlib
import json
import os
from pathlib import Path
import re
import struct
import subprocess
import sys
import threading
import urllib.error
import urllib.request
import uuid
import xml.etree.ElementTree as ET

BASE_URL = "https://sshzyu.com/v1"
SKILL_ROOT = Path(__file__).resolve().parent.parent
DEFAULT_ENV_FILE = SKILL_ROOT / ".env"
ENV_KEY = "SSHZYU_API_KEY"
CREDENTIAL_TARGET = "ClaudeCode/sshzyu-image/SSHZYU_API_KEY"
DEFAULT_MODEL = "gpt-image-2.6"
DEFAULT_SIZE = "1024x1024"
DEFAULT_QUALITY = "auto"
DEFAULT_FORMAT = "png"
MAX_RESPONSE = 128 * 1024 * 1024
MAX_INPUT = 32 * 1024 * 1024
MAX_JOBS = 64
MAX_WORKERS = 8
PRINT_LOCK = threading.Lock()
HTTP_HINTS = {
    401: "Key 无效或认证失败，请检查本技能目录中的 .env。",
    402: "余额不足，请检查服务商控制台；脚本不会自动充值。",
    403: "Key 所在分组未开通图片生成权限，请检查服务商控制台。",
    413: "请求体过大，请减少参考图数量或单图体积。",
    422: "参数不受支持，请核对模型、尺寸、质量和输出格式。",
    429: "限流，请稍后重试；本次没有自动重复提交。",
}

MODEL_CAPABILITIES = {
    "gpt-image-2.6": {
        "qualities": {"auto", "low", "medium", "high", "xhigh", "max"},
        "generate_formats": {"png", "jpeg", "webp", "gif", "svg"},
        "edit_formats": {"png", "jpeg", "webp"},
        "complexity": True,
        "gif": True,
        "svg": True,
        "markup": True,
    },
    "gpt-image-2.5": {
        "qualities": {"auto", "low", "medium", "high", "xhigh", "max"},
        "generate_formats": {"png", "jpeg", "webp"},
        "edit_formats": {"png", "jpeg", "webp"},
        "complexity": False,
        "gif": False,
        "svg": False,
        "markup": True,
    },
    "gpt-image-2": {
        "qualities": {"auto", "low", "medium", "high"},
        "generate_formats": {"png", "jpeg", "webp"},
        "edit_formats": {"png", "jpeg", "webp"},
        "complexity": False,
        "gif": False,
        "svg": False,
        "markup": False,
    },
}
UNKNOWN_GPT_IMAGE_CAPABILITIES = {
    "qualities": {"auto", "low", "medium", "high"},
    "generate_formats": {"png", "jpeg", "webp"},
    "edit_formats": {"png", "jpeg", "webp"},
    "complexity": False,
    "gif": False,
    "svg": False,
    "markup": False,
}


class ImageError(Exception):
    pass


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise ImageError("接口重定向已阻止，避免将认证信息发往其他地址。")


@dataclass
class Job:
    id: str
    prompt: str
    model: str = DEFAULT_MODEL
    size: str = DEFAULT_SIZE
    quality: str = DEFAULT_QUALITY
    output_format: str = DEFAULT_FORMAT
    n: int = 1
    background: str = "auto"
    moderation: str = "auto"
    output_compression: object = None
    complexity: object = None
    frame_count: object = None
    frame_duration_ms: object = None
    images: list = field(default_factory=list)
    mask: object = None
    markup: object = None
    safety_identifier: object = None
    timeout: float = 300

    @property
    def endpoint(self):
        return "/images/edits" if self.images else "/images/generations"


@dataclass
class JobResult:
    index: int
    job: Job
    status: str
    output_dir: object
    images: list = field(default_factory=list)
    usage: object = None
    warnings: list = field(default_factory=list)
    error: object = None
    duration_seconds: float = 0.0


def emit(message, error=False):
    with PRINT_LOCK:
        print(message, file=sys.stderr if error else sys.stdout, flush=True)


def redact(value, key=None):
    text = str(value)
    if key:
        text = text.replace(key, "[REDACTED]")
    text = re.sub(r"sk-[A-Za-z0-9_-]+", "[REDACTED]", text)
    text = re.sub(r"(?i)Bearer\s+\S+", "Bearer [REDACTED]", text)
    return text[:2000]


def credential_api():
    if os.name != "nt":
        raise ImageError("Windows 凭据管理器仅适用于 Windows；其他系统请使用 .env。")

    import ctypes
    from ctypes import wintypes

    class Credential(ctypes.Structure):
        _fields_ = [
            ("Flags", wintypes.DWORD),
            ("Type", wintypes.DWORD),
            ("TargetName", wintypes.LPWSTR),
            ("Comment", wintypes.LPWSTR),
            ("LastWritten", wintypes.FILETIME),
            ("CredentialBlobSize", wintypes.DWORD),
            ("CredentialBlob", ctypes.POINTER(ctypes.c_ubyte)),
            ("Persist", wintypes.DWORD),
            ("AttributeCount", wintypes.DWORD),
            ("Attributes", ctypes.c_void_p),
            ("TargetAlias", wintypes.LPWSTR),
            ("UserName", wintypes.LPWSTR),
        ]

    api = ctypes.WinDLL("advapi32", use_last_error=True)
    ptr = ctypes.POINTER(Credential)
    api.CredReadW.argtypes = [
        wintypes.LPCWSTR,
        wintypes.DWORD,
        wintypes.DWORD,
        ctypes.POINTER(ptr),
    ]
    api.CredReadW.restype = wintypes.BOOL
    api.CredFree.argtypes = [ctypes.c_void_p]
    api.CredFree.restype = None
    return api, Credential, ctypes


def read_credential():
    if os.name != "nt":
        return None
    api, credential, ctypes = credential_api()
    ptr = ctypes.POINTER(credential)()
    if not api.CredReadW(CREDENTIAL_TARGET, 1, 0, ctypes.byref(ptr)):
        error = ctypes.get_last_error()
        if error == 1168:
            return None
        raise ImageError(f"Windows 凭据读取失败，系统错误码 {error}。")
    try:
        return ctypes.string_at(
            ptr.contents.CredentialBlob,
            ptr.contents.CredentialBlobSize,
        ).decode("utf-8")
    finally:
        api.CredFree(ptr)


def clean_key(value):
    key = value.strip()
    if key.startswith("[") and key.endswith("]"):
        key = key[1:-1].strip()
    if not key or any(c.isspace() for c in key) or any(ord(c) < 32 for c in key):
        raise ImageError("Key 为空或包含空白/控制字符。")
    return key


def read_env_key(env_file):
    path = Path(env_file).expanduser().resolve()
    if not path.exists():
        return None
    key = None
    for raw_line in path.read_text(encoding="utf-8-sig").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].lstrip()
        if "=" not in line:
            continue
        name, value = line.split("=", 1)
        if name.strip() != ENV_KEY:
            continue
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in ("'", '"'):
            value = value[1:-1]
        key = clean_key(value)
    return key


def restrict_env_permissions(path):
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass
    if os.name != "nt":
        return
    user = os.environ.get("USERNAME")
    domain = os.environ.get("USERDOMAIN")
    principal = f"{domain}\\{user}" if domain and user else user
    if not principal:
        return
    try:
        subprocess.run(
            ["icacls", str(path), "/inheritance:r", "/grant:r", f"{principal}:(F)"],
            check=False,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=10,
        )
    except (OSError, subprocess.SubprocessError):
        pass


def write_env_key(env_file, key, replace=False):
    path = Path(env_file).expanduser().resolve()
    existing = read_env_key(path)
    if existing and not replace:
        raise ImageError(f"{path} 已包含 {ENV_KEY}；如需覆盖，明确使用 --replace-env。")
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = []
    replaced = False
    if path.exists() and existing:
        for raw_line in path.read_text(encoding="utf-8-sig").splitlines():
            stripped = raw_line.strip()
            candidate = stripped[7:].lstrip() if stripped.startswith("export ") else stripped
            if candidate.startswith(ENV_KEY + "="):
                if not replaced:
                    lines.append(f"{ENV_KEY}={clean_key(key)}")
                    replaced = True
                continue
            lines.append(raw_line)
    if not replaced:
        lines.append(f"{ENV_KEY}={clean_key(key)}")
    temp = path.with_name(path.name + f".tmp-{uuid.uuid4().hex}")
    try:
        temp.write_text("\n".join(lines).rstrip() + "\n", encoding="utf-8")
        os.replace(temp, path)
    finally:
        if temp.exists():
            temp.unlink()
    restrict_env_permissions(path)


def load_key(env_file):
    env_key = read_env_key(env_file)
    if env_key:
        return env_key, f"env:{Path(env_file).expanduser().resolve()}"
    env = os.environ.get(ENV_KEY)
    if env:
        return clean_key(env), "process environment"
    saved = read_credential()
    if saved:
        return clean_key(saved), "Windows Credential Manager fallback"
    key_file = SKILL_ROOT / "key.txt"
    if key_file.exists():
        return clean_key(key_file.read_text(encoding="utf-8-sig")), "legacy key.txt"
    raise ImageError(
        f"未配置 Key。把 {ENV_KEY}=... 写入 {DEFAULT_ENV_FILE}，"
        "或运行 --store-key 交互式创建 .env。"
    )


def request_json(path, key, payload=None, content_type="application/json", timeout=300):
    if path not in ("/models", "/images/generations", "/images/edits"):
        raise ImageError("不允许调用未列入规范的接口。")
    headers = {
        "Authorization": f"Bearer {key}",
        "Accept": "application/json",
        "User-Agent": "sshzyu-image-skill/1.1",
    }
    if payload is not None:
        headers["Content-Type"] = content_type
    req = urllib.request.Request(
        BASE_URL + path,
        data=payload,
        headers=headers,
        method="POST" if payload is not None else "GET",
    )
    opener = urllib.request.build_opener(NoRedirect())
    try:
        with opener.open(req, timeout=timeout) as response:
            body = response.read(MAX_RESPONSE + 1)
    except urllib.error.HTTPError as exc:
        raw = exc.read(64 * 1024)
        try:
            parsed = json.loads(raw)
            error = parsed.get("error", {})
            message = (
                error.get("message", "接口未提供 error.message")
                if isinstance(error, dict)
                else error
            )
        except (ValueError, AttributeError):
            message = "接口未返回 JSON 错误信息"
        hint = HTTP_HINTS.get(exc.code, "请求失败；未自动重试，以免重复计费。")
        raise ImageError(f"HTTP {exc.code}: {redact(message, key)}\n{hint}") from None
    except (urllib.error.URLError, TimeoutError, OSError) as exc:
        raise ImageError(
            f"网络请求失败: {redact(exc, key)}。"
            "未自动重试；超时后服务端可能仍在生成，请先核对用量。"
        ) from None
    if len(body) > MAX_RESPONSE:
        raise ImageError("响应超过 128 MiB 限制；停止处理，不自动重试。")
    try:
        result = json.loads(body)
    except (ValueError, UnicodeError):
        raise ImageError("接口未返回有效 JSON。") from None
    if not isinstance(result, dict):
        raise ImageError("接口响应必须是 JSON 对象。")
    if result.get("error"):
        raise ImageError("接口返回错误: " + redact(result["error"], key))
    return result


def local_name(tag):
    return tag.rsplit("}", 1)[-1].lower()


def parse_svg_size(data):
    if b"<!doctype" in data.lower() or b"<!entity" in data.lower():
        raise ImageError("SVG 包含 DTD/ENTITY，拒绝保存。")
    try:
        root = ET.fromstring(data)
    except (ET.ParseError, ValueError):
        raise ImageError("SVG XML 无效，不保存错误响应。") from None
    if local_name(root.tag) != "svg":
        raise ImageError("SVG 根节点不是 <svg>。")
    for element in root.iter():
        if local_name(element.tag) == "script":
            raise ImageError("SVG 包含 script，拒绝保存。")
        for attr, value in element.attrib.items():
            if local_name(attr) == "href" and value.strip() and not value.strip().startswith("#"):
                raise ImageError("SVG 包含外部引用，拒绝保存。")
    width = root.attrib.get("width")
    height = root.attrib.get("height")

    def number(value):
        if not value:
            return None
        match = re.match(r"^\s*([0-9]+(?:\.[0-9]+)?)", value)
        return int(float(match.group(1))) if match else None

    parsed_width = number(width)
    parsed_height = number(height)
    if (parsed_width is None or parsed_height is None) and root.attrib.get("viewBox"):
        pieces = re.split(r"[\s,]+", root.attrib["viewBox"].strip())
        if len(pieces) == 4:
            try:
                parsed_width = int(float(pieces[2]))
                parsed_height = int(float(pieces[3]))
            except ValueError:
                pass
    return parsed_width, parsed_height


def image_info(data):
    if data.startswith(b"\x89PNG\r\n\x1a\n") and len(data) >= 33 and data[12:16] == b"IHDR":
        width, height = struct.unpack(">II", data[16:24])
        return "png", width, height
    if data.startswith(b"\xff\xd8"):
        offset = 2
        while offset < len(data):
            if data[offset] != 255:
                break
            while offset < len(data) and data[offset] == 255:
                offset += 1
            if offset >= len(data):
                break
            marker = data[offset]
            offset += 1
            if marker in (0xD9, 0xDA):
                break
            if marker in (0x01, *range(0xD0, 0xD8)):
                continue
            if offset + 2 > len(data):
                break
            length = int.from_bytes(data[offset : offset + 2], "big")
            if length < 2 or offset + length > len(data):
                break
            if marker in (
                0xC0,
                0xC1,
                0xC2,
                0xC3,
                0xC5,
                0xC6,
                0xC7,
                0xC9,
                0xCA,
                0xCB,
                0xCD,
                0xCE,
                0xCF,
            ) and length >= 8:
                height, width = struct.unpack(">HH", data[offset + 3 : offset + 7])
                return "jpeg", width, height
            offset += length
    if len(data) >= 30 and data[:4] == b"RIFF" and data[8:12] == b"WEBP":
        kind = data[12:16]
        if kind == b"VP8X":
            width = int.from_bytes(data[24:27], "little") + 1
            height = int.from_bytes(data[27:30], "little") + 1
            return "webp", width, height
        if kind == b"VP8L" and data[20] == 0x2F:
            bits = int.from_bytes(data[21:25], "little")
            return "webp", (bits & 0x3FFF) + 1, ((bits >> 14) & 0x3FFF) + 1
        if kind == b"VP8 " and data[23:26] == b"\x9d\x01\x2a":
            width, height = struct.unpack("<HH", data[26:30])
            return "webp", width & 0x3FFF, height & 0x3FFF
    if len(data) >= 10 and data[:6] in (b"GIF87a", b"GIF89a"):
        width, height = struct.unpack("<HH", data[6:10])
        if width and height:
            return "gif", width, height
    if data.lstrip().startswith((b"<?xml", b"<svg", b"<SVG")):
        width, height = parse_svg_size(data)
        return "svg", width, height
    raise ImageError("图片魔数、SVG XML 或尺寸头无效，不保存错误响应。")


def billing_tier(size):
    if size == "auto":
        return "2K"
    longest = max(map(int, size.split("x")))
    return "1K" if longest <= 1024 else "2K" if longest <= 2048 else "4K"


def read_upload_file(path):
    resolved = Path(path).expanduser().resolve()
    if not resolved.is_file():
        raise ImageError(f"上传文件不存在: {resolved}")
    if resolved.stat().st_size > MAX_INPUT:
        raise ImageError(f"单张上传图片超过 32 MiB: {resolved}")
    data = resolved.read_bytes()
    fmt, width, height = image_info(data)
    if fmt == "svg":
        raise ImageError("SVG 不能作为参考图、mask 或 markup 上传。")
    return resolved, data, fmt, width, height


def multipart(fields, images, mask=None, markup=None):
    boundary = "sshzyu-" + uuid.uuid4().hex
    chunks = []
    for name, value in fields.items():
        if value is None:
            continue
        chunks.append(
            f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n'
            f"{value}\r\n".encode("utf-8")
        )

    uploaded = [read_upload_file(path) for path in images]
    if not uploaded:
        raise ImageError("编辑任务至少需要一张参考图。")
    first_width, first_height = uploaded[0][3], uploaded[0][4]
    total = 0
    files = [("image[]", item) for item in uploaded]
    if mask:
        mask_item = read_upload_file(mask)
        if mask_item[2] != "png":
            raise ImageError("mask 必须是 PNG。")
        if (mask_item[3], mask_item[4]) != (first_width, first_height):
            raise ImageError("mask 必须与第一张参考图宽高一致。")
        files.append(("mask", mask_item))
    if markup:
        markup_item = read_upload_file(markup)
        if markup_item[2] != "png":
            raise ImageError("markup 必须是透明 PNG。")
        if (markup_item[3], markup_item[4]) != (first_width, first_height):
            raise ImageError("markup 必须与第一张参考图宽高一致。")
        files.append(("markup", markup_item))

    for index, (name, item) in enumerate(files, 1):
        _, data, fmt, _, _ = item
        total += len(data)
        if total > MAX_RESPONSE:
            raise ImageError("上传素材合计超过 128 MiB 本地限制。")
        header = (
            f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"; '
            f'filename="input-{index}.{fmt}"\r\nContent-Type: image/{fmt}\r\n\r\n'
        )
        chunks.extend([header.encode("ascii"), data, b"\r\n"])
    chunks.append(f"--{boundary}--\r\n".encode("ascii"))
    return b"".join(chunks), f"multipart/form-data; boundary={boundary}"


def model_capabilities(model):
    for prefix in ("gpt-image-2.6", "gpt-image-2.5", "gpt-image-2"):
        if model.startswith(prefix):
            return MODEL_CAPABILITIES[prefix]
    return UNKNOWN_GPT_IMAGE_CAPABILITIES


def validate_job(job):
    if not job.prompt or not job.prompt.strip():
        raise ImageError(f"{job.id}: prompt 不能为空。")
    if len(job.prompt) > 32000:
        raise ImageError(f"{job.id}: prompt 超过 32000 字符。")
    if not job.model.startswith("gpt-image-"):
        raise ImageError(f"{job.id}: 仅支持 gpt-image- 模型家族。")
    if job.size != "auto" and not re.fullmatch(r"[1-9]\d*x[1-9]\d*", job.size):
        raise ImageError(f"{job.id}: size 必须是 auto 或正整数宽x高。")
    if not 1 <= job.n <= 10:
        raise ImageError(f"{job.id}: n 必须在 1 到 10 之间。")
    caps = model_capabilities(job.model)
    formats = caps["edit_formats"] if job.images else caps["generate_formats"]
    if job.output_format not in formats:
        endpoint_name = "编辑" if job.images else "生成"
        raise ImageError(
            f"{job.id}: {job.model} 的{endpoint_name}格式仅支持 "
            + ", ".join(sorted(formats))
        )
    if job.quality not in caps["qualities"]:
        raise ImageError(
            f"{job.id}: {job.model} 不支持 quality={job.quality}；可用 "
            + ", ".join(sorted(caps["qualities"]))
        )
    if job.output_compression is not None:
        if not 0 <= job.output_compression <= 100 or job.output_format not in ("jpeg", "webp"):
            raise ImageError(f"{job.id}: output_compression 为 0-100，且只用于 jpeg/webp。")
    if job.complexity is not None:
        if not caps["complexity"]:
            raise ImageError(f"{job.id}: complexity 仅适用于 gpt-image-2.6。")
        if job.complexity not in ("low", "medium", "high"):
            raise ImageError(f"{job.id}: complexity 必须是 low、medium 或 high。")
    if job.frame_count is not None or job.frame_duration_ms is not None:
        if job.output_format != "gif":
            raise ImageError(f"{job.id}: frame_count/frame_duration_ms 仅用于 GIF。")
        if not caps["gif"]:
            raise ImageError(f"{job.id}: {job.model} 不支持 GIF。")
    if job.frame_count is not None and not 2 <= job.frame_count <= 16:
        raise ImageError(f"{job.id}: frame_count 必须在 2 到 16 之间。")
    if job.frame_duration_ms is not None and not 20 <= job.frame_duration_ms <= 2000:
        raise ImageError(f"{job.id}: frame_duration_ms 必须在 20 到 2000 之间。")
    if job.output_format == "svg" and job.n != 1:
        raise ImageError(f"{job.id}: SVG 输出要求 n=1。")
    if len(job.images) > 16:
        raise ImageError(f"{job.id}: 参考图最多 16 张。")
    if (job.mask or job.markup) and not job.images:
        raise ImageError(f"{job.id}: mask/markup 必须与参考图一起使用。")
    if job.mask and job.markup:
        raise ImageError(f"{job.id}: mask 与 markup 不能同时使用。")
    if job.markup and not caps["markup"]:
        raise ImageError(f"{job.id}: {job.model} 不支持 markup。")
    if job.output_format in ("gif", "svg") and job.images:
        raise ImageError(f"{job.id}: 编辑接口不支持 {job.output_format.upper()} 输出。")
    if job.safety_identifier is not None and len(job.safety_identifier) > 128:
        raise ImageError(f"{job.id}: safety_identifier 最多 128 字符。")
    if job.timeout <= 0 or job.timeout > 1800:
        raise ImageError(f"{job.id}: timeout 必须大于 0 且不超过 1800 秒。")
    uploaded = [read_upload_file(path) for path in job.images]
    if uploaded:
        first_width, first_height = uploaded[0][3], uploaded[0][4]
        if job.mask:
            mask = read_upload_file(job.mask)
            if mask[2] != "png":
                raise ImageError(f"{job.id}: mask 必须是 PNG。")
            if (mask[3], mask[4]) != (first_width, first_height):
                raise ImageError(f"{job.id}: mask 必须与第一张参考图宽高一致。")
        if job.markup:
            markup = read_upload_file(job.markup)
            if markup[2] != "png":
                raise ImageError(f"{job.id}: markup 必须是 PNG。")
            if (markup[3], markup[4]) != (first_width, first_height):
                raise ImageError(f"{job.id}: markup 必须与第一张参考图宽高一致。")


def request_fields(job):
    fields = {
        "model": job.model,
        "prompt": job.prompt,
        "size": job.size,
        "quality": job.quality,
        "output_format": job.output_format,
        "background": job.background,
        "n": job.n,
        "moderation": job.moderation,
    }
    optional = {
        "output_compression": job.output_compression,
        "complexity": job.complexity,
        "frame_count": job.frame_count,
        "frame_duration_ms": job.frame_duration_ms,
        "safety_identifier": job.safety_identifier,
    }
    fields.update({key: value for key, value in optional.items() if value is not None})
    return fields


def decode_images(result, expected, output_format):
    items = result.get("data")
    if not isinstance(items, list) or not items:
        raise ImageError("响应缺少 data 图片数组。")
    warnings = []
    if len(items) != expected:
        warnings.append(
            f"请求 {expected} 张，服务端返回 {len(items)} 张；保留已返回图片，请核对计费。"
        )
    decoded = []
    for index, item in enumerate(items, 1):
        if not isinstance(item, dict) or not isinstance(item.get("b64_json"), str):
            warnings.append(f"第 {index} 张缺少 data[i].b64_json，已跳过。")
            continue
        try:
            raw = base64.b64decode(item["b64_json"], validate=True)
        except (binascii.Error, ValueError):
            warnings.append(f"第 {index} 张 base64 无效，已跳过。")
            continue
        try:
            fmt, width, height = image_info(raw)
        except ImageError as exc:
            warnings.append(f"第 {index} 张无效: {exc}")
            continue
        if fmt != output_format:
            warnings.append(
                f"第 {index} 张实际格式 {fmt} 与请求 {output_format} 不一致，已跳过。"
            )
            continue
        decoded.append((raw, fmt, width, height))
    if not decoded:
        raise ImageError("响应中没有可保存的有效图片。")
    return decoded, warnings


def save_images(result, job_dir, job, expected):
    decoded, warnings = decode_images(result, expected, job.output_format)
    directory = Path(job_dir).expanduser().resolve()
    directory.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ") + "-" + uuid.uuid4().hex[:8]
    records = []
    for index, (raw, fmt, width, height) in enumerate(decoded, 1):
        path = directory / f"{stamp}-{index:02d}.{fmt}"
        with path.open("xb") as stream:
            stream.write(raw)
        dimensions = f"{width}x{height}" if width and height else "unknown-size"
        records.append(
            {
                "path": str(path),
                "format": fmt,
                "width": width,
                "height": height,
                "bytes": len(raw),
                "sha256": hashlib.sha256(raw).hexdigest(),
            }
        )
        emit(f"[{job.id}] IMAGE: {path}")
        emit(f"[{job.id}] ACTUAL: {dimensions} ({fmt}, {len(raw)} bytes)")
        if job.size != "auto" and width and height and job.size != f"{width}x{height}":
            warnings.append("实际分辨率与请求不同；按服务端实际结果保存，未自动重试。")
            emit(f"[{job.id}] WARNING: 实际分辨率与请求 {job.size} 不同")
    return records, warnings


JOB_KEYS = {
    "id",
    "prompt",
    "prompt_file",
    "model",
    "size",
    "quality",
    "format",
    "output_format",
    "n",
    "background",
    "moderation",
    "output_compression",
    "complexity",
    "frame_count",
    "frame_duration_ms",
    "image",
    "images",
    "mask",
    "markup",
    "safety_identifier",
    "timeout",
    "mode",
}


def load_manifest(path):
    manifest_path = Path(path).expanduser().resolve()
    if not manifest_path.is_file():
        raise ImageError(f"manifest 不存在: {manifest_path}")
    try:
        data = json.loads(manifest_path.read_text(encoding="utf-8-sig"))
    except (ValueError, UnicodeError) as exc:
        raise ImageError(f"manifest JSON 无效: {redact(exc)}") from None
    if isinstance(data, list):
        return {"defaults": {}, "jobs": data}, manifest_path
    if not isinstance(data, dict):
        raise ImageError("manifest 顶层必须是对象或任务数组。")
    defaults = data.get("defaults", {})
    jobs = data.get("jobs", data.get("prompts"))
    if jobs is None and "prompt" in data:
        jobs = [data]
    if not isinstance(defaults, dict):
        raise ImageError("manifest.defaults 必须是对象。")
    if not isinstance(jobs, list):
        raise ImageError("manifest 必须包含 jobs 或 prompts 数组。")
    return {"defaults": defaults, "jobs": jobs}, manifest_path


def cli_overrides(args):
    names = (
        "model",
        "size",
        "quality",
        "output_format",
        "n",
        "background",
        "moderation",
        "output_compression",
        "complexity",
        "frame_count",
        "frame_duration_ms",
        "safety_identifier",
    )
    return {name: getattr(args, name) for name in names if getattr(args, name) is not None}


def normalize_job(raw, index, base_dir):
    if isinstance(raw, str):
        raw = {"prompt": raw}
    if not isinstance(raw, dict):
        raise ImageError(f"任务 {index} 必须是字符串或对象。")
    unknown = set(raw) - JOB_KEYS
    if unknown:
        raise ImageError(f"任务 {index} 包含未知字段: {', '.join(sorted(unknown))}")
    normalized = dict(raw)
    if "format" in normalized:
        if "output_format" in normalized:
            raise ImageError(f"任务 {index}: format 与 output_format 只能使用一个。")
        normalized["output_format"] = normalized.pop("format")
    if "image" in normalized:
        if "images" in normalized:
            raise ImageError(f"任务 {index}: image 与 images 只能使用一个。")
        normalized["images"] = normalized.pop("image")
    images = normalized.get("images")
    if isinstance(images, str):
        normalized["images"] = [images]
    elif images is None:
        normalized["images"] = []
    elif not isinstance(images, list) or not all(isinstance(item, str) for item in images):
        raise ImageError(f"任务 {index}: images 必须是路径数组。")
    else:
        normalized["images"] = [str(Path(base_dir, item).expanduser().resolve()) for item in images]

    for key in ("mask", "markup"):
        if normalized.get(key):
            normalized[key] = str(Path(base_dir, normalized[key]).expanduser().resolve())
    prompt_file = normalized.pop("prompt_file", None)
    if prompt_file:
        prompt_path = Path(base_dir, prompt_file).expanduser().resolve()
        if not prompt_path.is_file():
            raise ImageError(f"任务 {index}: prompt_file 不存在: {prompt_path}")
        normalized["prompt"] = prompt_path.read_text(encoding="utf-8-sig")
    if "timeout" in normalized:
        normalized["timeout"] = float(normalized["timeout"])
    return normalized


def merge_job(index, raw, manifest_defaults, overrides, base_dir, timeout):
    raw = normalize_job(raw, index, base_dir)
    merged = {
        "id": f"job-{index:02d}",
        "prompt": "",
        "model": DEFAULT_MODEL,
        "size": DEFAULT_SIZE,
        "quality": DEFAULT_QUALITY,
        "output_format": DEFAULT_FORMAT,
        "n": 1,
        "background": "auto",
        "moderation": "auto",
        "output_compression": None,
        "complexity": None,
        "frame_count": None,
        "frame_duration_ms": None,
        "images": [],
        "mask": None,
        "markup": None,
        "safety_identifier": None,
        "timeout": timeout,
    }
    for source in (manifest_defaults, overrides, raw):
        for key, value in source.items():
            if key in ("mode", "prompt_file"):
                continue
            if key == "images":
                merged["images"] = value
            elif key == "format":
                merged["output_format"] = value
            else:
                merged[key] = value
    merged["id"] = str(merged["id"])[:80]
    mode = raw.get("mode")
    if mode and mode not in ("generate", "edit"):
        raise ImageError(f"任务 {index}: mode 只能是 generate 或 edit。")
    if mode == "generate" and merged["images"]:
        raise ImageError(f"任务 {index}: mode=generate 不能包含 images。")
    if mode == "edit" and not merged["images"]:
        raise ImageError(f"任务 {index}: mode=edit 需要 images。")
    try:
        job = Job(**merged)
    except TypeError as exc:
        raise ImageError(f"任务 {index} 字段无效: {redact(exc)}") from None
    validate_job(job)
    return job


def build_jobs(args):
    overrides = cli_overrides(args)
    jobs = []
    if args.manifest:
        if args.prompt or args.prompt_file or args.image or args.mask or args.markup:
            raise ImageError(
                "--manifest 与 prompt、--prompt-file、--image、--mask、--markup 不能混用；"
                "请把任务内容写入 manifest。"
            )
        manifest, manifest_path = load_manifest(args.manifest)
        if not manifest["jobs"]:
            raise ImageError("manifest 没有任务。")
        if len(manifest["jobs"]) > MAX_JOBS:
            raise ImageError(f"单次最多 {MAX_JOBS} 个并发任务。")
        for index, raw in enumerate(manifest["jobs"], 1):
            jobs.append(
                merge_job(
                    index,
                    raw,
                    manifest["defaults"],
                    overrides,
                    manifest_path.parent,
                    args.timeout,
                )
            )
        return jobs

    prompt_sources = []
    if args.prompt:
        prompt_sources.append((f"prompt-{len(prompt_sources) + 1:02d}", args.prompt))
    for prompt_file in args.prompt_file or []:
        path = Path(prompt_file).expanduser().resolve()
        if not path.is_file():
            raise ImageError(f"提示词文件不存在: {path}")
        prompt_sources.append(
            (path.stem[:60] or f"prompt-{len(prompt_sources) + 1:02d}", path.read_text(encoding="utf-8-sig"))
        )
    if not prompt_sources:
        raise ImageError("请提供 prompt、--prompt-file 或 --manifest。")
    if len(prompt_sources) > MAX_JOBS:
        raise ImageError(f"单次最多 {MAX_JOBS} 个任务。")
    for index, (job_id, prompt) in enumerate(prompt_sources, 1):
        raw = {"id": job_id, "prompt": prompt, "images": args.image}
        if args.mask:
            raw["mask"] = args.mask
        if args.markup:
            raw["markup"] = args.markup
        jobs.append(
            merge_job(index, raw, {}, overrides, Path.cwd(), args.timeout)
        )
    return jobs


def slugify(value):
    safe = re.sub(r"[^A-Za-z0-9_-]+", "-", value).strip("-")
    return (safe or "job")[:40]


def print_plan(jobs):
    total_images = sum(job.n for job in jobs)
    endpoints = {"generation": 0, "edit": 0}
    for job in jobs:
        endpoints["edit" if job.images else "generation"] += 1
    emit(
        f"PLAN: jobs={len(jobs)}; requests={len(jobs)}; "
        f"expected_images={total_images}; generation_jobs={endpoints['generation']}; "
        f"edit_jobs={endpoints['edit']}"
    )
    for index, job in enumerate(jobs, 1):
        prompt_hash = hashlib.sha256(job.prompt.encode("utf-8")).hexdigest()[:12]
        emit(
            f"[{index:02d}] {job.id}: endpoint={job.endpoint}; model={job.model}; "
            f"n={job.n}; format={job.output_format}; images={len(job.images)}; "
            f"prompt_sha256={prompt_hash}"
        )


def execute_job(index, job, key, run_dir):
    started = datetime.now(timezone.utc)
    job_dir = run_dir / f"{index:02d}-{slugify(job.id)}"
    job_dir.mkdir(parents=True, exist_ok=True)
    emit(
        f"[{job.id}] REQUEST: {job.endpoint}; model={job.model}; size={job.size}; "
        f"billing_tier={billing_tier(job.size)}; n={job.n}; format={job.output_format}"
    )
    fields = request_fields(job)
    try:
        if job.images:
            payload, content_type = multipart(fields, job.images, job.mask, job.markup)
        else:
            payload = json.dumps(fields, ensure_ascii=False).encode("utf-8")
            content_type = "application/json"
        result = request_json(job.endpoint, key, payload, content_type, job.timeout)
        usage = result.get("usage")
        emit(f"[{job.id}] USAGE: {json.dumps(usage, ensure_ascii=False)}")
        if usage is None:
            emit(f"[{job.id}] WARNING: 服务端未返回 usage；不能推算真实费用。")
        records, warnings = save_images(result, job_dir, job, job.n)
        status = "partial" if warnings else "success"
        error = None
        if status == "partial":
            emit(f"[{job.id}] PARTIAL: {'; '.join(warnings)}")
    except (ImageError, OSError, UnicodeError) as exc:
        records = []
        warnings = []
        usage = None
        status = "error"
        error = redact(exc, key)
        emit(f"[{job.id}] ERROR: {error}", error=True)
    duration = (datetime.now(timezone.utc) - started).total_seconds()
    result_record = {
        "id": job.id,
        "status": status,
        "endpoint": BASE_URL + job.endpoint,
        "model": job.model,
        "prompt_sha256": hashlib.sha256(job.prompt.encode("utf-8")).hexdigest(),
        "requested_images": job.n,
        "returned_images": len(records),
        "size": job.size,
        "billing_tier": billing_tier(job.size),
        "quality": job.quality,
        "output_format": job.output_format,
        "complexity": job.complexity,
        "background": job.background,
        "moderation": job.moderation,
        "frame_count": job.frame_count,
        "frame_duration_ms": job.frame_duration_ms,
        "safety_identifier_present": bool(job.safety_identifier),
        "input_image_count": len(job.images),
        "duration_seconds": round(duration, 3),
        "usage": usage,
        "warnings": warnings,
        "error": error,
        "images": records,
    }
    with (job_dir / "job-result.json").open("w", encoding="utf-8") as stream:
        json.dump(result_record, stream, ensure_ascii=False, indent=2)
        stream.write("\n")
    return JobResult(
        index=index,
        job=job,
        status=status,
        output_dir=job_dir,
        images=records,
        usage=usage,
        warnings=warnings,
        error=error,
        duration_seconds=duration,
    )


def run_jobs(jobs, key, out_dir, workers):
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
    run_dir = Path(out_dir).expanduser().resolve() / f"run-{stamp}-{uuid.uuid4().hex[:8]}"
    run_dir.mkdir(parents=True, exist_ok=False)
    started = datetime.now(timezone.utc)
    future_map = {}
    results = []
    emit(f"RUN: {run_dir}")
    if len(jobs) == 1:
        results.append(execute_job(1, jobs[0], key, run_dir))
    else:
        with ThreadPoolExecutor(max_workers=workers, thread_name_prefix="sshzyu-image") as executor:
            for index, job in enumerate(jobs, 1):
                future_map[executor.submit(execute_job, index, job, key, run_dir)] = (index, job)
            for future in as_completed(future_map):
                index, job = future_map[future]
                try:
                    results.append(future.result())
                except Exception as exc:
                    results.append(
                        JobResult(
                            index=index,
                            job=job,
                            status="error",
                            output_dir=None,
                            error=redact(exc, key),
                        )
                    )
    results.sort(key=lambda item: item.index)
    duration = (datetime.now(timezone.utc) - started).total_seconds()
    success = sum(item.status == "success" for item in results)
    partial = sum(item.status == "partial" for item in results)
    failed = sum(item.status == "error" for item in results)
    summary = {
        "run_dir": str(run_dir),
        "created": datetime.now(timezone.utc).isoformat(),
        "workers": workers,
        "jobs_requested": len(jobs),
        "jobs_succeeded": success,
        "jobs_partial": partial,
        "jobs_failed": failed,
        "images_requested": sum(job.n for job in jobs),
        "images_saved": sum(len(item.images) for item in results),
        "duration_seconds": round(duration, 3),
        "results": [
            {
                "id": item.job.id,
                "status": item.status,
                "output_dir": str(item.output_dir) if item.output_dir else None,
                "images": [record["path"] for record in item.images],
                "usage": item.usage,
                "warnings": item.warnings,
                "error": item.error,
            }
            for item in results
        ],
    }
    summary_path = run_dir / "batch-summary.json"
    with summary_path.open("w", encoding="utf-8") as stream:
        json.dump(summary, stream, ensure_ascii=False, indent=2)
        stream.write("\n")
    emit(
        f"SUMMARY: jobs={success}/{len(jobs)} succeeded; partial={partial}; "
        f"images={summary['images_saved']}/{summary['images_requested']}; "
        f"failed={failed}; elapsed={duration:.2f}s; exit={2 if failed == 0 and partial else 1 if failed else 0}"
    )
    emit(f"RECORD: {summary_path}")
    if failed:
        return 1
    return 2 if partial else 0


def parser():
    p = argparse.ArgumentParser(
        description=(
            "SSHZYU 批量生图/参考图编辑；单 prompt 多图使用 n，"
            "多 prompt 使用并发任务，默认不自动重试。"
        )
    )
    p.add_argument("prompt", nargs="?", help="单个提示词；多提示词使用 --prompt-file 或 --manifest")
    p.add_argument(
        "--prompt-file",
        action="append",
        help="UTF-8 提示词文件，可重复；每个文件作为独立并发任务",
    )
    p.add_argument("--manifest", help="JSON 批处理清单；每个任务并发执行")
    p.add_argument("--image", action="append", default=[], help="参考图，可重复，最多 16 张")
    p.add_argument("--mask", help="可选 PNG 蒙版")
    p.add_argument("--markup", help="可选透明 PNG 标记层，gpt-image-2.5/2.6")
    p.add_argument("--size")
    p.add_argument(
        "--quality",
        choices=["auto", "low", "medium", "high", "xhigh", "max"],
    )
    p.add_argument(
        "--format",
        dest="output_format",
        choices=["png", "jpeg", "webp", "gif", "svg"],
    )
    p.add_argument("--n", type=int, help="每个任务的图片数，1-10")
    p.add_argument("--model", help="默认 gpt-image-2.6；以 --models 返回为准")
    p.add_argument("--complexity", choices=["low", "medium", "high"])
    p.add_argument("--frame-count", type=int)
    p.add_argument("--frame-duration-ms", type=int)
    p.add_argument("--safety-identifier")
    p.add_argument("--background", choices=["auto", "transparent", "opaque"])
    p.add_argument("--moderation", choices=["auto", "low"])
    p.add_argument("--output-compression", type=int)
    p.add_argument("--out", default="./sshzyu-images")
    p.add_argument("--workers", type=int, default=4, help=f"并发任务数，1-{MAX_WORKERS}")
    p.add_argument("--timeout", type=float, default=300)
    p.add_argument("--env-file", default=str(DEFAULT_ENV_FILE))
    p.add_argument("--dry-run", action="store_true", help="只展开并校验任务，不读 Key、不联网")
    p.add_argument("--confirm", action="store_true", help="用户已明确确认生图计划后才可执行")
    group = p.add_mutually_exclusive_group()
    group.add_argument("--models", action="store_true", help="读取模型列表，不生图")
    group.add_argument("--doctor", action="store_true", help="检查 .env 与凭据来源，不联网")
    group.add_argument("--store-key", action="store_true", help="交互式写入 .env，不回显 Key")
    group.add_argument(
        "--migrate-credential-to-env",
        action="store_true",
        help="把旧 Windows 凭据迁移到 .env，不打印 Key",
    )
    p.add_argument("--replace-env", action="store_true")
    return p


def main(argv=None):
    args = parser().parse_args(argv)
    key = None
    try:
        if not 1 <= args.workers <= MAX_WORKERS:
            raise ImageError(f"--workers 必须在 1 到 {MAX_WORKERS} 之间。")
        if args.replace_env and not (args.store_key or args.migrate_credential_to_env):
            raise ImageError("--replace-env 只能与 --store-key 或迁移命令一起使用。")
        if args.store_key:
            key = getpass.getpass("SSHZYU API Key（不回显）: ")
            write_env_key(args.env_file, key, args.replace_env)
            emit(f"已写入 {Path(args.env_file).expanduser().resolve()}；Key 未输出。")
            return 0
        if args.migrate_credential_to_env:
            saved = read_credential()
            if not saved:
                raise ImageError("没有找到旧 Windows 凭据，无法迁移。")
            write_env_key(args.env_file, saved, args.replace_env)
            emit(f"已迁移到 {Path(args.env_file).expanduser().resolve()}；Key 未输出。")
            return 0
        if args.doctor:
            path = Path(args.env_file).expanduser().resolve()
            env_key = read_env_key(path)
            source = f"env:{path}" if env_key else "not-loaded"
            if not env_key and os.environ.get(ENV_KEY):
                source = "process environment"
            elif not env_key and read_credential():
                source = "Windows Credential Manager fallback"
            emit(
                json.dumps(
                    {
                        "python": sys.version.split()[0],
                        "env_file": str(path),
                        "env_file_exists": path.exists(),
                        "key_present": bool(env_key)
                        or bool(os.environ.get(ENV_KEY))
                        or bool(read_credential()),
                        "credential_source": source,
                        "endpoint": BASE_URL,
                        "network_tested": False,
                    },
                    ensure_ascii=False,
                )
            )
            return 0
        if args.models:
            key, source = load_key(args.env_file)
            result = request_json("/models", key, timeout=args.timeout)
            emit(json.dumps(result, ensure_ascii=False, indent=2))
            return 0
        jobs = build_jobs(args)
        print_plan(jobs)
        if args.dry_run:
            return 0
        if not args.confirm:
            raise ImageError(
                "真实生图前必须完成 preflight 并向用户确认；"
                "确认后用 --dry-run 核对计划，再在真实命令加 --confirm。"
            )
        key, source = load_key(args.env_file)
        workers = min(args.workers, len(jobs))
        emit(f"CREDENTIAL: {source}; workers={workers}; no automatic retry")
        return run_jobs(jobs, key, args.out, workers)
    except (ImageError, OSError, UnicodeError) as exc:
        emit("ERROR: " + redact(exc, key), error=True)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
