import argparse
import datetime
import json
import os
import pathlib
import re
import sys
import urllib.parse
import urllib.request


ROOT = pathlib.Path(__file__).resolve().parents[2]
REPOSITORY = "ravicc02/sshzyu"
API = f"https://api.github.com/repos/{REPOSITORY}"
VERSION = re.compile(r"(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)-r([1-9]\d*)\Z")


def parse_version(value):
    match = VERSION.fullmatch(value)
    if not match:
        raise ValueError("Invalid custom version")
    numbers = tuple(map(int, match.groups()))
    if any(number > 2**64 - 1 for number in numbers):
        raise ValueError("Version component overflow")
    return numbers


def should_publish(current, latest):
    return latest is None or parse_version(current) > parse_version(latest)


class SafeRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, file_pointer, code, message, headers, new_url):
        target = urllib.parse.urlsplit(new_url)
        if target.scheme != "https" or target.username or target.netloc not in {
            "api.github.com", "github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com"
        }:
            raise ValueError("Asset redirect rejected")
        redirected = super().redirect_request(request, file_pointer, code, message, headers, new_url)
        if target.netloc != "api.github.com":
            redirected.remove_header("Authorization")
        return redirected


def fetch(path, limit, binary=False):
    token = os.environ.get("GITHUB_TOKEN", "")
    if not token:
        raise ValueError("GitHub authentication is required")
    request = urllib.request.Request(
        API + path,
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/octet-stream" if binary else "application/vnd.github+json",
            "User-Agent": "sshzy-release-policy",
        },
    )
    opener = urllib.request.build_opener(SafeRedirect())
    with opener.open(request, timeout=30) as response:
        data = response.read(limit + 1)
    if len(data) > limit:
        raise ValueError("Release response too large")
    return data if binary else json.loads(data)


def published_releases():
    candidates = []
    for page in range(1, 21):
        entries = fetch(f"/releases?per_page=100&page={page}", 8 * 1024 * 1024)
        if not isinstance(entries, list):
            raise ValueError("Invalid release list")
        for entry in entries:
            if entry.get("draft") or entry.get("prerelease"):
                continue
            tag = entry.get("tag_name", "")
            if not tag.startswith("sshzy-v"):
                continue
            parse_version(tag.removeprefix("sshzy-v"))
            candidates.append(entry)
        if len(entries) < 100:
            return sorted(candidates, key=lambda entry: parse_version(entry["tag_name"].removeprefix("sshzy-v")), reverse=True)
    raise ValueError("Release pagination limit exceeded")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--download-previous", type=pathlib.Path)
    options = parser.parse_args()
    if os.environ.get("GITHUB_REPOSITORY") != REPOSITORY:
        raise ValueError("Unexpected release repository")
    version = (ROOT / "backend/cmd/server/VERSION").read_text().strip()
    baseline = json.loads((ROOT / "upstream-baseline.json").read_text())
    parse_version(version)
    if baseline["local_build_version"] != version:
        raise ValueError("Baseline and version differ")
    releases = published_releases()
    latest = releases[0] if releases else None
    if options.download_previous:
        if latest:
            options.download_previous.mkdir(parents=True, exist_ok=False)
            assets = {asset["name"]: asset for asset in latest["assets"]}
            for name, maximum in (("release-manifest.json", 512 * 1024), ("release-manifest.sig", 256)):
                asset = assets.get(name)
                if not asset or not isinstance(asset.get("id"), int) or asset["id"] <= 0:
                    raise ValueError("Previous signed release is incomplete")
                (options.download_previous / name).write_bytes(fetch(f"/releases/assets/{asset['id']}", maximum, binary=True))
        return 0
    latest_version = latest["tag_name"].removeprefix("sshzy-v") if latest else None
    publish = should_publish(version, latest_version)
    output = os.environ.get("GITHUB_OUTPUT")
    if not output:
        raise ValueError("This command must run inside the publishing job")
    date = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    with open(output, "a", encoding="utf-8") as stream:
        stream.write(f"publish={'true' if publish else 'false'}\nversion={version}\ndate={date}\n")
    print("New immutable custom version ready for verification." if publish else "No version increment; existing releases will not be replaced.")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception:
        print("Unable to verify the custom publishing policy; publishing is blocked.", file=sys.stderr)
        sys.exit(1)
