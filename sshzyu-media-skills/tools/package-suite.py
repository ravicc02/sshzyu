#!/usr/bin/env python3
"""Package the SSHZYU media skill suite into one installable archive.

The suite root is ``sshzyu-media-skills/``; installable skills live in
``sshzyu-media-skills/skills/``. The archive keeps the ``skills/`` layer, so the
top level is ``skills/<name>/`` plus the suite-level README, VERSION, LICENSE
and documentation.
"""

import argparse
import hashlib
import io
from pathlib import Path, PurePosixPath
import re
import sys
import tempfile
import zipfile

SUITE_ROOT = Path(__file__).resolve().parents[1]
REPO_ROOT = SUITE_ROOT.parent
SOURCE = SUITE_ROOT / "skills"
DEFAULT_OUTPUT = REPO_ROOT / "docs" / "install" / "sshzyu-media.zip"
SUITE_FILES = ("AGENTS.md", "CHANGELOG.md", "LICENSE", "README.md", "VERSION", "docs/architecture.md")
SKILLS = (
    "sshzyu-media", "sshzyu-image", "sshzyu-video",
    "short-drama", "short-drama-assets", "short-drama-breakdown",
    "short-drama-develop", "short-drama-edit", "short-drama-image-prompts",
    "short-drama-novel-analyze", "short-drama-produce", "short-drama-review",
    "short-drama-storyboard", "short-drama-video-prompts", "short-drama-write",
)
REQUIRED = {
    "sshzyu-media": ("ROUTING.md", "scripts/sshzyu_media.py", "scripts/sshzyu_image.py", "scripts/sshzyu_drama.py"),
    "sshzyu-image": ("scripts/sshzyu_image.py", "references/image/api_reference.md", "references/image/parameter-matrix.md"),
    "sshzyu-video": ("scripts/sshzyu_video.py", "scripts/media_common.py", "references/api_reference.md", "references/parameter-matrix.md"),
    "short-drama": ("scripts/project_tool.py", "assets/dashboard/index.html"),
    "short-drama-produce": ("scripts/production_tool.py", "references/adapter-contract.md"),
}
FORBIDDEN = {".env", "key.txt", "__pycache__", "node_modules", ".git", "tests", "dist", "build"}
SUFFIXES = {".pyc", ".zip", ".skill", ".sha256"}
TEXT = {".example", ".gitignore", ".json", ".jsonl", ".md", ".py", ".sh", ".toml", ".txt", ".yaml", ".yml", ".js", ".css", ".html", ".tsx", ".ts"}
LINK = re.compile(r"\]\(([^)#]+)(?:#[^)]*)?\)")
SECRET_NAME = re.compile(r"(?:^|[._-])(?:secret|credential|token|password|private|apikey|api_key)(?:[._-]|$)", re.IGNORECASE)


def excluded(relative):
    parts = relative.parts
    return any(
        part in FORBIDDEN or (part.startswith(".env.") and part != ".env.example") or (part != ".env.example" and SECRET_NAME.search(part))
        for part in parts
    ) or relative.suffix.lower() in SUFFIXES or relative.name.startswith("test_")


class PackageError(Exception):
    pass


def validate_source():
    for name in SUITE_FILES:
        if not (SUITE_ROOT / name).is_file():
            raise PackageError(f"缺少套件级文件: {name}")
    present = {p.name for p in SOURCE.iterdir() if (p / "SKILL.md").is_file()}
    if not set(SKILLS) <= present:
        raise PackageError(f"缺少套件入口: {sorted(set(SKILLS) - present)}")
    for name in SKILLS:
        root = SOURCE / name
        for relative in ("SKILL.md", *REQUIRED.get(name, ())):
            if not (root / relative).is_file():
                raise PackageError(f"缺少必需资源: {name}/{relative}")
        text = (root / "SKILL.md").read_text(encoding="utf-8")
        front = re.match(r"^---\r?\n(.*?)\r?\n---\r?\n", text, re.DOTALL)
        if not front or not re.search(rf"^name:\s*{re.escape(name)}\s*$", front[1], re.MULTILINE):
            raise PackageError(f"frontmatter name 与目录名不符: {name}")
        for path in root.rglob("*.md"):
            if excluded(path.relative_to(root)):
                continue
            for target in LINK.findall(path.read_text(encoding="utf-8")):
                if target.startswith(("/", "http:", "https:", "mailto:", "#")):
                    continue
                resolved = (path.parent / target).resolve()
                if not resolved.is_file() or not resolved.is_relative_to(SOURCE.resolve()):
                    raise PackageError(f"失效或包外相对链接: {path.relative_to(SUITE_ROOT)} -> {target}")
                relative = resolved.relative_to(SOURCE.resolve())
                if relative.parts[0] not in SKILLS or excluded(relative):
                    raise PackageError(f"指向未打包资源: {path.relative_to(SUITE_ROOT)} -> {target}")


def normalized_bytes(path):
    data = path.read_bytes()
    if path.name == ".gitignore" or path.suffix.lower() in TEXT:
        data = data.replace(b"\r\n", b"\n").replace(b"\r", b"\n")
    return data


def source_files():
    files = [(PurePosixPath(name), normalized_bytes(SUITE_ROOT / name)) for name in SUITE_FILES]
    for name in SKILLS:
        root = SOURCE / name
        for path in root.rglob("*"):
            if path.is_symlink():
                raise PackageError(f"禁止打包符号链接: {path}")
            if not path.is_file():
                continue
            relative = path.relative_to(SOURCE)
            if excluded(relative):
                continue
            files.append((PurePosixPath("skills") / relative, normalized_bytes(path)))
    return sorted(files, key=lambda item: item[0].as_posix())


def build_package():
    validate_source()
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for relative, data in source_files():
            info = zipfile.ZipInfo(relative.as_posix(), date_time=(2026, 11, 17, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.create_system = 3
            info.external_attr = (0o644 & 0xFFFF) << 16
            archive.writestr(info, data, compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)
    payload = buffer.getvalue()
    with zipfile.ZipFile(io.BytesIO(payload)) as archive:
        names = archive.namelist()
        parts = {PurePosixPath(name).parts for name in names}
        entries = {p[1] for p in parts if len(p) == 3 and p[0] == "skills" and p[2] == "SKILL.md"}
        if entries != set(SKILLS) or len(names) != len(set(names)):
            raise PackageError("发布包入口名单不完整或存在重名")
        for name in names:
            parts = PurePosixPath(name).parts
            if not parts or name.startswith("/") or ".." in parts:
                raise PackageError(f"无效 ZIP 条目: {name}")
            if parts[0] == "skills":
                if len(parts) < 2 or parts[1] not in SKILLS:
                    raise PackageError(f"未声明的技能目录: {name}")
            elif PurePosixPath(name).as_posix() not in SUITE_FILES:
                raise PackageError(f"未声明的套件文件: {name}")
        with tempfile.TemporaryDirectory(prefix="sshzyu-skills-check-") as tmp:
            install_root = Path(tmp)
            # Explicitly refuse pre-existing same-name entries, including original short-drama.
            (install_root / "skills" / "short-drama").mkdir(parents=True)
            conflicts = [skill for skill in SKILLS if (install_root / "skills" / skill).exists()]
            if conflicts != ["short-drama"]:
                raise PackageError("同名技能冲突检查异常")
            (install_root / "skills" / "short-drama").rmdir()
            for member in archive.infolist():
                destination = install_root.joinpath(*PurePosixPath(member.filename).parts)
                if member.is_dir():
                    destination.mkdir(parents=True, exist_ok=True)
                    continue
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_bytes(archive.read(member))
            installed = {p.name for p in (install_root / "skills").iterdir() if (p / "SKILL.md").is_file()}
            if installed != set(SKILLS):
                raise PackageError("隔离解包后的独立入口名单不一致")
    return payload


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args(argv)
    try:
        payload = build_package()
        output = args.output.resolve()
        digest = hashlib.sha256(payload).hexdigest()
        if args.check:
            if not output.is_file() or output.read_bytes() != payload:
                print(f"STALE: {output}", file=sys.stderr)
                return 1
            print(f"OK: {output}; SHA256: {digest}")
        else:
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_bytes(payload)
            print(f"PACKAGE: {output}; SIZE: {len(payload)}; SHA256: {digest}")
        return 0
    except (OSError, PackageError, UnicodeError, zipfile.BadZipFile) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
