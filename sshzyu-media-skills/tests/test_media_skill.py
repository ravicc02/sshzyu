"""Behavior and package tests for the SSHZYU media skill suite."""

import importlib.util
import io
import json
import shutil
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock
import zipfile

SUITE_ROOT = Path(__file__).resolve().parents[1]
SKILL_ROOT = SUITE_ROOT / "skills" / "sshzyu-image"
sys.path.insert(0, str(SKILL_ROOT / "scripts"))

import sshzyu_image as cli

PACKAGE_SCRIPT = SUITE_ROOT / "tools" / "package-suite.py"
package_spec = importlib.util.spec_from_file_location("sshzyu_image_package", PACKAGE_SCRIPT)
if package_spec is None or package_spec.loader is None:
    raise RuntimeError("cannot load skill package builder")
skill_package = importlib.util.module_from_spec(package_spec)
package_spec.loader.exec_module(skill_package)

PNG_BASE64 = (
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8"
    "AAusB9Y9Z0GQAAAAASUVORK5CYII="
)


def image_response(count):
    return {
        "created": 1,
        "data": [{"b64_json": PNG_BASE64} for _ in range(count)],
        "usage": {"total_tokens": count},
    }


class BatchStatusTests(unittest.TestCase):
    def test_complete_response_returns_zero(self):
        with tempfile.TemporaryDirectory() as output:
            job = cli.Job(id="complete", prompt="test", n=1, size="auto")
            with mock.patch.object(cli, "request_json", return_value=image_response(1)):
                code = cli.run_jobs([job], "test-key", output, 1)
            summary_path = next(Path(output).rglob("batch-summary.json"))
            summary = json.loads(summary_path.read_text(encoding="utf-8"))
            self.assertEqual(code, 0)
            self.assertEqual(summary["jobs_succeeded"], 1)
            self.assertEqual(summary["jobs_partial"], 0)

    def test_short_delivery_is_partial_and_returns_two(self):
        with tempfile.TemporaryDirectory() as output:
            job = cli.Job(id="short", prompt="test", n=2, size="auto")
            with mock.patch.object(cli, "request_json", return_value=image_response(1)):
                code = cli.run_jobs([job], "test-key", output, 1)
            summary_path = next(Path(output).rglob("batch-summary.json"))
            summary = json.loads(summary_path.read_text(encoding="utf-8"))
            job_result_path = next(Path(output).rglob("job-result.json"))
            job_result = json.loads(job_result_path.read_text(encoding="utf-8"))
            self.assertEqual(code, 2)
            self.assertEqual(summary["jobs_succeeded"], 0)
            self.assertEqual(summary["jobs_partial"], 1)
            self.assertEqual(job_result["status"], "partial")
            self.assertEqual(job_result["requested_images"], 2)
            self.assertEqual(job_result["returned_images"], 1)

    def test_request_error_returns_one(self):
        with tempfile.TemporaryDirectory() as output:
            job = cli.Job(id="failed", prompt="test", n=1, size="auto")
            with mock.patch.object(
                cli,
                "request_json",
                side_effect=cli.ImageError("request failed"),
            ):
                code = cli.run_jobs([job], "test-key", output, 1)
            summary_path = next(Path(output).rglob("batch-summary.json"))
            summary = json.loads(summary_path.read_text(encoding="utf-8"))
            self.assertEqual(code, 1)
            self.assertEqual(summary["jobs_failed"], 1)
            self.assertEqual(summary["jobs_partial"], 0)


class PackageContractTests(unittest.TestCase):
    def test_package_is_deterministic_and_normalizes_text_line_endings(self):
        first = skill_package.build_package()
        second = skill_package.build_package()
        self.assertEqual(first, second)

        with zipfile.ZipFile(io.BytesIO(first)) as archive:
            skill_md = archive.read("skills/sshzyu-media/SKILL.md")
            self.assertNotIn(b"\r\n", skill_md)
            self.assertNotIn(b"\r", skill_md)
            names = set(archive.namelist())
            entries = {
                name.split("/")[1]
                for name in names
                if name.startswith("skills/") and name.count("/") == 2 and name.endswith("/SKILL.md")
            }
            self.assertEqual(set(skill_package.SKILLS), entries)
            self.assertEqual(len(names), len(archive.namelist()))
            self.assertIn("README.md", names)
            self.assertIn("VERSION", names)
            self.assertIn("skills/sshzyu-video/scripts/sshzyu_video.py", names)
            self.assertIn("skills/sshzyu-media/scripts/sshzyu_drama.py", names)
            self.assertIn("skills/sshzyu-video/references/api_reference.md", names)
            self.assertFalse(any("/tests/" in name or name.endswith("/.env") for name in names))
            for info in archive.infolist():
                self.assertEqual(3, info.create_system)

    def test_package_ignores_unrelated_skill_and_rejects_wrong_frontmatter(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "skills"
            shutil.copytree(skill_package.SOURCE, root, ignore=shutil.ignore_patterns("__pycache__"))
            with mock.patch.object(skill_package, "SOURCE", root):
                (root / "unexpected" / "SKILL.md").parent.mkdir()
                (root / "unexpected" / "SKILL.md").write_text("---\nname: unexpected\n---\n")
                self.assertNotIn(
                    "skills/unexpected/SKILL.md",
                    dict((p.as_posix(), data) for p, data in skill_package.source_files()),
                )
                skill_package.validate_source()
                skill = root / "sshzyu-video" / "SKILL.md"
                skill.write_text(
                    skill.read_text(encoding="utf-8").replace("name: sshzyu-video", "name: wrong"),
                    encoding="utf-8",
                )
                with self.assertRaises(skill_package.PackageError):
                    skill_package.validate_source()

    def test_package_excludes_secret_and_runtime_files(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "skills"
            shutil.copytree(skill_package.SOURCE, root, ignore=shutil.ignore_patterns("__pycache__"))
            for name in (".env.production", "service-token.json", "private.pem", "api_key.txt", "cache.pyc"):
                (root / "sshzyu-image" / name).write_text("do not publish", encoding="utf-8")
            with mock.patch.object(skill_package, "SOURCE", root):
                names = {path.as_posix() for path, _ in skill_package.source_files()}
            self.assertFalse(
                any(
                    name.startswith("skills/sshzyu-image/")
                    and name.split("/")[-1] in {".env.production", "service-token.json", "private.pem", "api_key.txt", "cache.pyc"}
                    for name in names
                )
            )

    def test_installation_layout_is_parallel_and_conflicts_are_not_overwritten(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "install"
            root.mkdir()
            payload = skill_package.build_package()
            with zipfile.ZipFile(io.BytesIO(payload)) as archive:
                install_root = root / "skills"
                for entry in skill_package.SKILLS:
                    self.assertFalse((install_root / entry).exists(), "同名目录必须先经冲突检查")
                archive.extractall(root)
            self.assertEqual(
                set(skill_package.SKILLS),
                {p.name for p in (root / "skills").iterdir() if (p / "SKILL.md").is_file()},
            )
            existing = root / "skills" / "short-drama" / "SKILL.md"
            existing.write_text("原版技能，不得覆盖", encoding="utf-8")
            self.assertEqual("原版技能，不得覆盖", existing.read_text(encoding="utf-8"))
            self.assertTrue((root / "skills" / "short-drama").exists(), "已有原版不可静默覆盖")


if __name__ == "__main__":
    unittest.main()
