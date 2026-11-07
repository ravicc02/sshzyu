"""Behavior tests for the sshzyu-image batch status contract."""

import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

REPO_ROOT = Path(__file__).resolve().parents[2]
SKILL_ROOT = REPO_ROOT / "skills" / "sshzyu-image"
sys.path.insert(0, str(SKILL_ROOT / "scripts"))

import sshzyu_image as cli

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


if __name__ == "__main__":
    unittest.main()
