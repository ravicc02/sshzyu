#!/usr/bin/env python3
"""Check, stage, deploy and verify the standalone public documentation site."""

import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import hashlib
from html.parser import HTMLParser
import json
from pathlib import Path, PurePosixPath
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
import uuid
from urllib.parse import quote, unquote, urlsplit
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[1]
REQUIRED = (
    "index.html", "content.md", "docs-category.css",
    "image/index.html", "image/content.md", "vedio/index.html", "vedio/content.md",
    "install/sshzyu-image.zip",
)
SSH = ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=15", "us-server"]

# Paths and host are intentionally fixed to the repository's deployment boundary.
ACTIVATE = r'''set -euo pipefail
release="$1"
[[ "$release" =~ ^[0-9]{8}T[0-9]{6}Z-[a-f0-9]{12}-[a-f0-9]{8}$ ]]
exec 9>/opt/.sshzyu-docs-publish.lock
flock -n 9 || { echo "Another docs deployment is active" >&2; exit 1; }
stage="/opt/sshzyu-docs-releases/$release"
backup="/opt/sshzyu-docs-backups/$release"
target=/opt/sshzyu-docs
test -d "$target" && test ! -L "$target"
test ! -e "$backup"
# Existing symlinks could redirect writes outside the documentation directory.
test -z "$(find "$target" -type l -print -quit)"
cd "$stage/docs"
sha256sum -c ../SHA256SUMS
mkdir -p /opt/sshzyu-docs-backups
cp -a "$target" "$backup"
test -f "$backup/index.html"
# Supporting files and child pages are ready before the root entry changes.
rsync -a --partial --info=progress2 --exclude=/index.html "$stage/docs/" "$target/"
install -m 644 "$stage/docs/index.html" "$target/.index.html.$release"
mv -f "$target/.index.html.$release" "$target/index.html"
cd "$target"
sha256sum -c "$stage/SHA256SUMS"
printf 'BACKUP=%s\nRELEASE=%s\n' "$backup" "$stage"
'''


FINALIZE = r'''set -euo pipefail
release="$1"
[[ "$release" =~ ^[0-9]{8}T[0-9]{6}Z-[a-f0-9]{12}-[a-f0-9]{8}$ ]]
exec 9>/opt/.sshzyu-docs-publish.lock
flock -n 9
stage="/opt/sshzyu-docs-releases/$release"
cd /opt/sshzyu-docs
sha256sum -c "$stage/SHA256SUMS"
test -s "$stage/verification.json"
install -m 644 "$stage/deployment.json" "/opt/.sshzyu-docs-deployed.$release.json"
mv -f "/opt/.sshzyu-docs-deployed.$release.json" /opt/sshzyu-docs-deployed.json
'''


def git(*args):
    return subprocess.check_output(["git", "-C", str(ROOT), *args])


def tracked_docs():
    paths = git("ls-files", "-z", "--", "docs").decode("utf-8").split("\0")
    return [PurePosixPath(p).relative_to("docs").as_posix() for p in paths if p]


class Links(HTMLParser):
    def __init__(self):
        super().__init__()
        self.urls = []

    def handle_starttag(self, tag, attrs):
        self.urls.extend(value for key, value in attrs if key in ("href", "src") and value)


def local_reference(root, source, value):
    url = urlsplit(value)
    if url.scheme or url.netloc or not url.path:
        return None
    path = unquote(url.path)
    if path.startswith("/"):
        if not path.startswith("/docs/"):
            return None  # Other main-site routes have separate owners.
        target = root / path[len("/docs/"):]
    else:
        target = source.parent / path
    target = target.resolve()
    if not target.is_relative_to(root.resolve()):
        raise ValueError(f"{source.name}: link escapes docs: {value}")
    if target.is_dir():
        target /= "index.html"
    return target


def check(root, names):
    root = Path(root)
    files = set(names)
    for name in REQUIRED:
        if name not in files:
            raise ValueError(f"Required docs file is not tracked: {name}")
    for name in files:
        path = root / name
        if path.is_symlink() or not path.is_file() or not path.resolve().is_relative_to(root.resolve()):
            raise ValueError(f"Not a regular documentation file: {name}")
        if any(c in name for c in "\r\n\\") or name.startswith("-"):
            raise ValueError(f"Unsupported docs filename: {name!r}")
    # Follow pages, fetched Markdown and styles. Unlinked operational notes do
    # not become entry pages merely because their Markdown is tracked.
    pending = [root / name for name in files if name.endswith(".html")]
    seen = set()
    links = 0
    while pending:
        source = pending.pop()
        if source in seen:
            continue
        seen.add(source)
        text = source.read_text(encoding="utf-8-sig")
        if source.suffix == ".html":
            parser = Links()
            parser.feed(text)
            urls = parser.urls + re.findall(r'''fetch\(\s*['"]([^'"]+)['"]''', text)
        elif source.suffix == ".css":
            urls = re.findall(r'''url\(\s*['"]?([^'"\s)]+)''', text)
        else:
            text = re.sub(r"```.*?```|~~~.*?~~~", "", text, flags=re.S)
            urls = re.findall(r"!?\[[^\]]*\]\(([^\s)]+)", text)
        for value in urls:
            target = local_reference(root, source, value)
            if target is None:
                continue
            links += 1
            relative = target.relative_to(root.resolve()).as_posix()
            if relative not in files or not target.is_file():
                raise ValueError(f"{source.relative_to(root)}: missing or untracked reference: {value}")
            if target.suffix in (".html", ".md", ".css"):
                pending.append(target)
    return {"files": len(files), "links_checked": links}


def stage(output):
    if git("status", "--porcelain=v1", "--untracked-files=no", "--", "docs").strip():
        raise ValueError("Commit docs changes before staging a deployment")
    names = tracked_docs()
    summary = check(ROOT / "docs", names)
    sha = git("rev-parse", "HEAD").decode().strip()
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    release = f"{stamp}-{sha[:12]}-{uuid.uuid4().hex[:8]}"
    output = Path(output).resolve() if output else Path(tempfile.mkdtemp(prefix="sshzy-docs-"))
    if output.exists() and any(output.iterdir()):
        raise ValueError("Staging output must be an empty directory")
    output.mkdir(parents=True, exist_ok=True)
    files = []
    for name in sorted(names):
        target = output / "docs" / name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(ROOT / "docs" / name, target)
        files.append({"path": name, "sha256": hashlib.sha256(target.read_bytes()).hexdigest()})
    check(output / "docs", names)
    if git("status", "--porcelain=v1", "--untracked-files=no", "--", "docs").strip() or git("rev-parse", "HEAD").decode().strip() != sha:
        raise ValueError("Source changed while staging docs; prepare a fresh deployment")
    manifest = {"release": release, "source_sha": sha, "docs_tree": git("rev-parse", "HEAD:docs").decode().strip(), "file_count": summary["files"], "links_checked": summary["links_checked"], "files": files}
    (output / "deployment.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    (output / "SHA256SUMS").write_text("".join(f"{f['sha256']}  {f['path']}\n" for f in files), encoding="utf-8", newline="\n")
    print(json.dumps({"stage": str(output), "release": release, **summary}, ensure_ascii=False))
    return output


def load_manifest(bundle):
    manifest = json.loads((Path(bundle) / "deployment.json").read_text(encoding="utf-8"))
    names = set()
    for item in manifest["files"]:
        name = item["path"]
        path = PurePosixPath(name)
        if path.is_absolute() or ".." in path.parts or name in names or "\\" in name or not re.fullmatch(r"[a-f0-9]{64}", item["sha256"]):
            raise ValueError("Invalid documentation manifest")
        names.add(name)
    if not set(REQUIRED).issubset(names):
        raise ValueError("Incomplete documentation manifest")
    return manifest


def verify(bundle, base_url):
    base_url = base_url.rstrip("/") + "/"
    parsed = urlsplit(base_url)
    if parsed.scheme not in ("http", "https") or parsed.query or parsed.fragment:
        raise ValueError("Use an HTTP(S) documentation base URL without query or fragment")
    manifest = load_manifest(bundle)

    def fetch(item):
        name, expected = item["path"], item["sha256"]
        request = Request(base_url + quote(name, safe="/"), headers={"Cache-Control": "no-cache", "User-Agent": "sshzy-docs-release/1.0"})
        with urlopen(request, timeout=30) as response:
            if response.status != 200 or hashlib.sha256(response.read()).hexdigest() != expected:
                raise ValueError(f"HTTP content differs from deployment: {name}")
        return name

    with ThreadPoolExecutor(max_workers=4) as pool:
        list(pool.map(fetch, manifest["files"]))
    # Check nginx directory entries as well as directly named index.html files.
    for item in manifest["files"]:
        if item["path"].endswith("index.html"):
            fetch({**item, "path": item["path"][:-len("index.html")]})
    result = {"release": manifest["release"], "source_sha": manifest["source_sha"], "base_url": base_url,
              "files_verified": len(manifest["files"]), "verified_at": datetime.now(timezone.utc).isoformat()}
    (Path(bundle) / "verification.json").write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(result))
    return result


def rsync_command(source, destination):
    options = ["-a", "--partial", "--info=progress2", "--chmod=D755,F644"]
    if shutil.which("rsync"):
        return ["rsync", *options, "-e", "ssh -o BatchMode=yes -o ConnectTimeout=15", str(source) + "/", destination]
    if sys.platform != "win32" or not shutil.which("wsl"):
        raise ValueError("rsync is required (Windows supports WSL Ubuntu rsync)")

    def wsl_path(path):
        path = Path(path).resolve()
        if not path.drive or path.drive.startswith("\\"):
            raise ValueError("WSL transfer requires a local Windows drive")
        return "/mnt/" + path.drive[0].lower() + path.as_posix()[2:]

    ssh = shutil.which("ssh")
    if not ssh:
        raise ValueError("Native SSH client is required to reuse the local SSH configuration")
    remote_shell = shlex.quote(wsl_path(ssh)) + " -o BatchMode=yes -o ConnectTimeout=15"
    return ["wsl", "-d", "Ubuntu", "--exec", "rsync", *options, "-e", remote_shell, wsl_path(source) + "/", destination]


def deploy(output):
    bundle = stage(output)
    manifest = load_manifest(bundle)
    release = manifest["release"]
    remote = "/opt/sshzyu-docs-releases/" + release
    subprocess.run([*SSH, f"test ! -e {remote} && mkdir -p {remote}"], check=True)
    subprocess.run(rsync_command(bundle, "us-server:" + remote + "/"), check=True)
    subprocess.run([*SSH, "bash -s -- " + shlex.quote(release)], input=ACTIVATE, text=True, check=True)
    verify(bundle, "https://sshzyu.com/docs/")
    subprocess.run(rsync_command(bundle, "us-server:" + remote + "/"), check=True)
    subprocess.run([*SSH, "bash -s -- " + shlex.quote(release)], input=FINALIZE, text=True, check=True)
    print("Docs deployed. Backup: /opt/sshzyu-docs-backups/" + release)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("check", help="Validate tracked public docs and local references")
    for command in ("stage", "deploy"):
        commands.add_parser(command).add_argument("--output", type=Path)
    verifier = commands.add_parser("verify", help="Compare every public file with a staged manifest")
    verifier.add_argument("--bundle", type=Path, required=True)
    verifier.add_argument("--base-url", default="https://sshzyu.com/docs/")
    args = parser.parse_args()
    try:
        if args.command == "check":
            print(json.dumps(check(ROOT / "docs", tracked_docs())))
        elif args.command == "stage":
            stage(args.output)
        elif args.command == "verify":
            verify(args.bundle, args.base_url)
        else:
            deploy(args.output)
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(f"Docs release failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
