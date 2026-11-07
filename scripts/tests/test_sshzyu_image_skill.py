"""Behavior tests for the sshzyu-image batch status contract."""

import json
import importlib.util
import io
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock
import zipfile

REPO_ROOT = Path(__file__).resolve().parents[2]
SKILL_ROOT = REPO_ROOT / "skills" / "sshzyu-image"
sys.path.insert(0, str(SKILL_ROOT / "scripts"))

import sshzyu_image as cli

PACKAGE_SCRIPT = REPO_ROOT / "scripts" / "package-sshzyu-image-skill.py"
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
            skill_md = archive.read("sshzyu-image/SKILL.md")
            self.assertNotIn(b"\r\n", skill_md)
            self.assertNotIn(b"\r", skill_md)
            for info in archive.infolist():
                self.assertEqual(3, info.create_system)


if __name__ == "__main__":
    unittest.main()
