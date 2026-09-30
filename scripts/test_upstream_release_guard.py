"""官方 Release 推送门禁的无网络回归测试。"""

import importlib.util
import io
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

MODULE_PATH = Path(__file__).with_name("upstream_release_guard.py")
spec = importlib.util.spec_from_file_location("upstream_release_guard", MODULE_PATH)
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)

BASE = {
    "upstream_repository": "Wei-Shaw/sub2api",
    "upstream_release": "v0.2.11",
    "upstream_commit": "a" * 40,
    "upstream_release_url": "https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.11",
    "local_build_version": "0.2.11-r6",
}


def release(tag="v0.2.11"):
    return {
        "tag_name": tag,
        "draft": False,
        "prerelease": False,
        "published_at": "2026-09-30T07:06:51Z",
        "html_url": f"https://github.com/Wei-Shaw/sub2api/releases/tag/{tag}",
    }


class GuardTests(unittest.TestCase):
    def test_baseline_requires_matching_version_and_official_repo(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "backend/cmd/server").mkdir(parents=True)
            (root / "backend/cmd/server/VERSION").write_text("0.2.11-r6\n")
            (root / "upstream-baseline.json").write_text(json.dumps(BASE))
            with patch.object(guard, "REPO", root):
                self.assertEqual(guard.read_baseline()["upstream_commit"], "a" * 40)
                (root / "backend/cmd/server/VERSION").write_text("0.2.11-r7\n")
                with self.assertRaises(guard.Unknown):
                    guard.read_baseline()

    def test_invalid_versions_are_unknown(self):
        for tag in ("0.2.11", "v0.2.rc", "v0.2.12-rc1", "v0.2.011", "v0.3.1extra"):
            with self.subTest(tag=tag), self.assertRaises(guard.Unknown):
                guard.parse_version(tag)

    @patch.object(guard, "official_tag_sha", return_value="a" * 40)
    @patch.object(guard, "latest_release", return_value=release())
    @patch.object(guard, "read_baseline", return_value=BASE)
    def test_same_release_passes_for_all_refs(self, read, _release, _sha):
        guard.check(["b" * 40, "c" * 40])
        self.assertEqual(read.call_count, 2)

    @patch.object(guard, "official_tag_sha", return_value="a" * 40)
    @patch.object(guard, "latest_release", return_value=release("v0.2.12"))
    @patch.object(guard, "read_baseline", return_value=BASE)
    def test_new_release_blocks_all_refs(self, *_mocks):
        with self.assertRaisesRegex(guard.Unknown, "新的官方稳定 Release"):
            guard.check(["b" * 40, "c" * 40])

    @patch.object(guard, "official_tag_sha", return_value="b" * 40)
    @patch.object(guard, "latest_release", return_value=release())
    @patch.object(guard, "read_baseline", return_value=BASE)
    def test_baseline_tag_sha_mismatch_blocks(self, *_mocks):
        with self.assertRaisesRegex(guard.Unknown, "commit 不匹配"):
            guard.check(["b" * 40])

    @patch.object(guard, "latest_release", side_effect=guard.Unknown("HTTP 403"))
    @patch.object(guard, "read_baseline", return_value=BASE)
    def test_unknown_network_blocks(self, *_mocks):
        with self.assertRaises(guard.Unknown):
            guard.check(["b" * 40])

    def test_latest_release_selects_highest_stable_semver(self):
        older = release("v0.2.11")
        newer = release("v0.2.12")
        prerelease = dict(release("v0.3.0-rc1"), prerelease=True)
        response = io.BytesIO(json.dumps([older, prerelease, newer]).encode())
        response.status = 200
        with patch.object(guard.urllib.request, "urlopen", return_value=response):
            self.assertEqual(guard.latest_release()["tag_name"], "v0.2.12")

    def test_latest_release_rejects_prerelease_and_invalid_json(self):
        for payload in ([dict(release(), prerelease=True)], [dict(release(), draft=True)],
                        [release("v0.2.12-rc1")], {"invalid": True}):
            with self.subTest(payload=payload):
                response = io.BytesIO(json.dumps(payload).encode())
                response.status = 200
                with patch.object(guard.urllib.request, "urlopen", return_value=response):
                    with self.assertRaises(guard.Unknown):
                        guard.latest_release()

    def test_latest_release_uses_next_page_at_100_items(self):
        first = io.BytesIO(json.dumps([release("v0.2.11")] * 100).encode())
        first.status = 200
        second = io.BytesIO(json.dumps([release("v0.2.12")]).encode())
        second.status = 200
        with patch.object(guard.urllib.request, "urlopen", side_effect=[first, second]) as fetch:
            self.assertEqual(guard.latest_release()["tag_name"], "v0.2.12")
            self.assertEqual(fetch.call_count, 2)

    @patch.object(guard, "git", side_effect=[" M backend/internal/a.go", ""])
    def test_dirty_backend_cannot_claim_clean_build(self, _git):
        with self.assertRaisesRegex(guard.Unknown, "未提交"):
            guard.validate_clean_build()

    def test_tag_sha_uses_official_remote_and_peels_annotated_tag(self):
        def fake_git(*args):
            if args[:2] == ("remote", "get-url"):
                return "https://github.com/Wei-Shaw/sub2api.git"
            if args[0] == "ls-remote":
                return f"{'b' * 40}\trefs/tags/v0.2.11\n{'a' * 40}\trefs/tags/v0.2.11^{{}}"
            raise AssertionError(args)

        with patch.object(guard, "git", side_effect=fake_git):
            self.assertEqual(guard.official_tag_sha("v0.2.11"), "a" * 40)
        with patch.object(guard, "git", return_value="https://github.com/elsewhere/sub2api.git"):
            with self.assertRaises(guard.Unknown):
                guard.official_tag_sha("v0.2.11")

    @patch.object(guard, "check")
    def test_pre_push_checks_each_commit_and_rejects_deletion(self, check):
        lines = (f"refs/heads/main {'a' * 40} refs/heads/main {'b' * 40}\n"
                 f"refs/heads/test {'c' * 40} refs/heads/test {'b' * 40}\n")
        with patch.object(guard, "git", return_value=""):
            guard.pre_push("origin", "https://github.com/ravicc02/sshzyu.git", io.StringIO(lines))
            check.assert_called_once_with(["a" * 40, "c" * 40])
            with self.assertRaises(guard.Unknown):
                guard.pre_push("origin", "unused", io.StringIO(f"(delete) {guard.ZERO} refs/heads/main {'b' * 40}\n"))


if __name__ == "__main__":
    unittest.main()
