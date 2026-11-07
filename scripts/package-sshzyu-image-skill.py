#!/usr/bin/env python3
"""Build the installable sshzyu-image ZIP without local secrets."""

import argparse
import hashlib
import io
from pathlib import Path
from pathlib import PurePosixPath
import re
import sys
import zipfile

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "skills" / "sshzyu-image"
DEFAULT_OUTPUT = ROOT / "docs" / "install" / "sshzyu-image.zip"
REQUIRED = {
    "SKILL.md",
    "scripts/sshzyu_image.py",
    "references/api_reference.md",
    "references/parameter-matrix.md",
    "references/models/README.md",
    "references/models/gpt-image-2.md",
    "references/models/gpt-image-2.5.md",
    "references/models/gpt-image-2.6.md",
    ".env.example",
}
EXCLUDED_PARTS = {".env", "key.txt", "__pycache__", "tests"}
EXCLUDED_SUFFIXES = {".pyc", ".skill", ".zip", ".sha256"}


class PackageError(Exception):
    pass


def validate_source():
    missing = [relative for relative in sorted(REQUIRED) if not (SOURCE / relative).is_file()]
    if missing:
        raise PackageError("缺少必需文件: " + ", ".join(missing))
    skill_md = (SOURCE / "SKILL.md").read_text(encoding="utf-8")
    frontmatter_match = re.match(r"^---\r?\n(.*?)\r?\n---\r?\n", skill_md, re.DOTALL)
    if not frontmatter_match:
        raise PackageError("SKILL.md 缺少有效 frontmatter。")
    frontmatter = frontmatter_match.group(1)
    if not re.search(r"^name:\s*sshzyu-image\s*$", frontmatter, re.MULTILINE):
        raise PackageError("SKILL.md 的 name 必须是 sshzyu-image。")
    description_match = re.search(
        r'^description:\s*"((?:[^"\\]|\\.)*)"\s*$',
        frontmatter,
        re.MULTILINE,
    )
    if not description_match:
        raise PackageError("SKILL.md 的 description 必须使用双引号单行格式。")
    description = description_match.group(1)
    if not description or len(description) > 1024 or re.search(r"[<>]", description):
        raise PackageError("SKILL.md 的 description 为空、超长或包含尖括号。")


def source_files():
    files = []
    for path in SOURCE.rglob("*"):
        if not path.is_file():
            continue
        relative = path.relative_to(SOURCE)
        if any(part in EXCLUDED_PARTS for part in relative.parts):
            continue
        if path.suffix.lower() in EXCLUDED_SUFFIXES:
            continue
        files.append((relative, path.read_bytes()))
    files.sort(key=lambda item: item[0].as_posix())
    return files


def build_package():
    validate_source()
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for relative, data in source_files():
            info = zipfile.ZipInfo(
                filename=f"sshzyu-image/{relative.as_posix()}",
                date_time=(2026, 11, 6, 0, 0, 0),
            )
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = (0o644 & 0xFFFF) << 16
            archive.writestr(info, data, compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)
    payload = buffer.getvalue()
    names = zipfile.ZipFile(io.BytesIO(payload)).namelist()
    for name in names:
        if any(part in {".env", "key.txt", "__pycache__"} for part in PurePosixPath(name).parts):
            raise PackageError("打包结果包含禁止发布的本地凭据或缓存。")
    return payload


def write_outputs(payload, output):
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_bytes(payload)
    return hashlib.sha256(payload).hexdigest()


def package_matches(payload, output):
    if not output.is_file():
        return False
    return hashlib.sha256(output.read_bytes()).digest() == hashlib.sha256(payload).digest()


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--check", action="store_true", help="只验证已提交包是否最新")
    args = parser.parse_args(argv)
    output = args.output.resolve()
    try:
        payload = build_package()
        digest = hashlib.sha256(payload).hexdigest()
        if args.check:
            if not package_matches(payload, output):
                print(f"STALE: {output}", file=sys.stderr)
                return 1
            print(f"OK: {output}")
            print(f"SHA256: {digest}")
            return 0
        digest = write_outputs(payload, output)
        print(f"PACKAGE: {output}")
        print(f"SIZE: {len(payload)} bytes")
        print(f"SHA256: {digest}")
        return 0
    except (OSError, PackageError, UnicodeError, zipfile.BadZipFile) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
