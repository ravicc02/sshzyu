#!/usr/bin/env python3
"""推送前核对官方稳定 Release；任何不确定状态均阻止向 origin 推送。"""

import argparse
import json
import re
import subprocess
import sys
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
RELEASE_API = "https://api.github.com/repos/Wei-Shaw/sub2api/releases"
OFFICIAL_REMOTE = re.compile(
    r"^(?:https://github\.com/|git@github\.com:|ssh://git@github\.com/)Wei-Shaw/sub2api(?:\.git)?/?$",
    re.IGNORECASE,
)
VERSION = re.compile(r"v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)\Z")
LOCAL_VERSION = re.compile(r"(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)-r([1-9]\d*)\Z")
SHA = re.compile(r"[0-9a-f]{40}\Z")
ZERO = "0" * 40


class Unknown(RuntimeError):
    """无法证明上游状态或本地基线。"""


def git(*args):
    proc = subprocess.run(
        ["git", "-C", str(REPO), *args], capture_output=True, text=True,
        encoding="utf-8", errors="replace", timeout=30, check=False,
    )
    if proc.returncode:
        raise Unknown(f"git {' '.join(args[:3])} 失败：{proc.stderr.strip()[:300]}")
    return proc.stdout.strip()


def parse_version(value):
    match = VERSION.fullmatch(value)
    if not match or not value.startswith("v"):
        raise Unknown(f"不合法的官方 Release tag：{value!r}")
    return tuple(map(int, match.groups()))


def read_baseline(ref=None):
    try:
        content = git("show", f"{ref}:upstream-baseline.json") if ref else (REPO / "upstream-baseline.json").read_text(encoding="utf-8")
        data = json.loads(content)
        version = git("show", f"{ref}:backend/cmd/server/VERSION") if ref else (REPO / "backend/cmd/server/VERSION").read_text(encoding="utf-8").strip()
        tag = data["upstream_release"]
        commit = data["upstream_commit"]
        local = data["local_build_version"]
        match = LOCAL_VERSION.fullmatch(local)
        if (data["upstream_repository"] != "Wei-Shaw/sub2api"
                or not VERSION.fullmatch(tag) or not tag.startswith("v")
                or not SHA.fullmatch(commit) or not match
                or local != version or tuple(map(int, match.groups()[:3])) != parse_version(tag)
                or data["upstream_release_url"] != f"https://github.com/Wei-Shaw/sub2api/releases/tag/{tag}"):
            raise Unknown("基线记录、版本文件、仓库身份或上游 Release URL 不一致")
        return data
    except (OSError, KeyError, ValueError, json.JSONDecodeError) as exc:
        raise Unknown(f"无法读取有效的基线记录：{exc}") from exc


def official_tag_sha(tag):
    # 不依赖本地可变标签；只接受官方 remote 直接报告的 tag 对象及其剥离后的 commit。
    remote = git("remote", "get-url", "upstream")
    if not OFFICIAL_REMOTE.fullmatch(remote):
        raise Unknown(f"upstream remote 不是官方仓库：{remote}")
    lines = git("ls-remote", "--tags", "upstream", f"refs/tags/{tag}", f"refs/tags/{tag}^{{}}")
    refs = {}
    for line in lines.splitlines():
        fields = line.split()
        if len(fields) == 2:
            refs[fields[1]] = fields[0]
    direct = refs.get(f"refs/tags/{tag}")
    peeled = refs.get(f"refs/tags/{tag}^{{}}", direct)
    if not direct or not peeled or not SHA.fullmatch(peeled):
        raise Unknown(f"无法确认官方标签 {tag} 的 commit SHA")
    return peeled


def latest_release():
    """枚举全部可见 Release，不把 GitHub 的 latest 指针当成最高 SemVer。"""
    candidates = []
    page = 1
    try:
        while True:
            req = urllib.request.Request(
                f"{RELEASE_API}?per_page=100&page={page}",
                headers={"Accept": "application/vnd.github+json", "User-Agent": "sshzy-upstream-release-guard"},
            )
            with urllib.request.urlopen(req, timeout=15) as resp:
                if resp.status != 200:
                    raise Unknown(f"GitHub Release API 状态：{resp.status}")
                data = json.load(resp)
            if not isinstance(data, list) or not data:
                if page == 1 or not isinstance(data, list):
                    raise Unknown("官方 Release 列表为空或结构无效")
                break
            for item in data:
                if not isinstance(item, dict) or not isinstance(item.get("draft"), bool) or not isinstance(item.get("prerelease"), bool):
                    raise Unknown("Release 列表元数据无效")
                if item["draft"] or item["prerelease"]:
                    continue
                tag = item.get("tag_name")
                if not isinstance(tag, str) or not isinstance(item.get("html_url"), str) or not item.get("published_at"):
                    raise Unknown("稳定 Release 元数据不完整")
                if item["html_url"] != f"https://github.com/Wei-Shaw/sub2api/releases/tag/{tag}":
                    raise Unknown("稳定 Release URL 不匹配")
                candidates.append((parse_version(tag), item))
            if len(data) < 100:
                break
            page += 1
            if page > 100:
                raise Unknown("官方 Release 超过 100 页，无法完成全量核验")
        if not candidates:
            raise Unknown("未找到官方稳定 Release")
        return max(candidates, key=lambda candidate: candidate[0])[1]
    except (urllib.error.URLError, TimeoutError, OSError, KeyError, ValueError, TypeError, json.JSONDecodeError) as exc:
        raise Unknown(f"GitHub Release 状态无法确认：{exc}") from exc


def check(refs):
    # 先核对本地每个将推送的 ref 的不可变快照，而非工作区临时文件。
    baselines = [(ref or "工作区", read_baseline(ref)) for ref in refs]
    release = latest_release()
    latest_tag = release["tag_name"]
    latest_sha = official_tag_sha(latest_tag)
    now = datetime.now(timezone.utc).isoformat(timespec="seconds")
    sha_by_tag = {}
    for ref, baseline in baselines:
        base_tag = baseline["upstream_release"]
        if base_tag not in sha_by_tag:
            sha_by_tag[base_tag] = latest_sha if base_tag == latest_tag else official_tag_sha(base_tag)
        base_sha = sha_by_tag[base_tag]
        print(f"[{now}] {ref}: 当前基线 {base_tag} ({base_sha}), 本地 {baseline['local_build_version']}; "
              f"官方最新 {latest_tag} ({latest_sha}), 发布于 {release['published_at']}, {release['html_url']}")
        if base_sha != baseline["upstream_commit"]:
            raise Unknown(f"{ref}: 基线记录的官方 tag commit 不匹配")
        if parse_version(latest_tag) > parse_version(base_tag):
            raise Unknown(f"{ref}: 发现新的官方稳定 Release {latest_tag}；暂停推送，需先由用户决定是否合并")
        if parse_version(latest_tag) < parse_version(base_tag):
            raise Unknown(f"{ref}: 当前记录的基线比官方最新稳定 Release 新，无法确认状态")
    print("检查通过：当前没有比待推送基线更新的官方稳定 Release；这不代表已完成测试或已获推送授权。")


def validate_clean_build(build_target="backend"):
    """拒绝将未提交的镜像输入标记为 HEAD 的正式发布构建。"""
    paths = ["backend", "upstream-baseline.json"]
    if build_target == "root":
        paths += ["Dockerfile", ".dockerignore", "frontend", "docs/legal", "deploy/docker-entrypoint.sh"]
    elif build_target != "backend":
        raise Unknown(f"未知的构建入口：{build_target}")
    tracked = git("status", "--porcelain", "--untracked-files=no", "--", *paths)
    untracked = git("ls-files", "--others", "--exclude-standard", "--", *paths)
    if tracked or untracked:
        raise Unknown(f"{build_target} 镜像输入含未提交文件；先确认归属并提交，再使用该提交构建发布镜像")
    if read_baseline() != read_baseline("HEAD"):
        raise Unknown("HEAD 中的基线记录与工作区不一致")


def pre_push(remote_name, remote_url, stream):
    if remote_name != "origin" and remote_url != git("remote", "get-url", "origin"):
        return
    refs = []
    for line in stream:
        fields = line.split()
        if len(fields) != 4:
            raise Unknown("pre-push 输入格式无效")
        local_ref, local_sha, remote_ref, _remote_sha = fields
        if local_sha == ZERO:  # 删除远端 ref 也需核验当前工作分支的基线
            raise Unknown(f"{remote_ref}: 删除 ref 不在自动检查范围内，暂停推送")
        if not SHA.fullmatch(local_sha):
            raise Unknown(f"{local_ref}: 本地提交 SHA 无效")
        refs.append((local_ref, local_sha))
    if refs:
        print(f"待推送到 {remote_name}: {', '.join(ref for ref, _ in refs)}")
        check([sha for _ref, sha in refs])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pre-push", nargs=2, metavar=("REMOTE", "URL"))
    parser.add_argument("--validate-clean-build", action="store_true", help="核实正式构建所用源码与 HEAD 提交一致")
    parser.add_argument("--build-target", choices=("backend", "root"), default="backend", help="正式镜像构建入口")
    opts = parser.parse_args()
    try:
        if opts.pre_push:
            pre_push(*opts.pre_push, sys.stdin)
        elif opts.validate_clean_build:
            validate_clean_build(opts.build_target)
            print(f"{opts.build_target} 镜像输入与 HEAD 中的版本记录一致，所检查路径无未提交改动")
        else:
            print(f"当前分支：{git('branch', '--show-current')}；HEAD：{git('rev-parse', 'HEAD')}")
            print(f"相对 origin/main 待推送提交：{git('rev-list', '--count', 'origin/main..HEAD')}")
            check([None])
    except (Unknown, subprocess.TimeoutExpired) as exc:
        print(f"上游检查结果：未知或不允许推送。{exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
