from __future__ import annotations

from pathlib import Path
import json
import sys
import tempfile
import unittest
from unittest.mock import patch


SKILL_ROOT = Path(__file__).resolve().parents[1]
SCRIPT_ROOT = SKILL_ROOT / "scripts"
sys.path.insert(0, str(SCRIPT_ROOT))
sys.path.insert(1, str(SKILL_ROOT.parent / "sshzyu-media" / "scripts"))

import sshzyu_video as video  # noqa: E402


def make_job(**overrides):
    values = {
        "id": "shot-01",
        "prompt": "一只小猫沿着海边慢慢走动，镜头稳定",
        "model": "minimax_h3",
        "seconds": 4,
        "resolution": "480p",
        "aspect_ratio": "16:9",
    }
    values.update(overrides)
    return video.VideoJob(**values)


class VideoJobTests(unittest.TestCase):
    def test_valid_job(self):
        video.validate_video_job(make_job())

    def test_requires_prompt_and_model(self):
        with self.assertRaises(video.VideoError):
            video.validate_video_job(make_job(prompt=""))
        with self.assertRaises(video.VideoError):
            video.validate_video_job(make_job(model=""))

    def test_seconds_and_resolution_validation(self):
        for seconds in (0, -1, 61):
            with self.subTest(seconds=seconds):
                with self.assertRaises(video.VideoError):
                    video.validate_video_job(make_job(seconds=seconds))
        with self.assertRaises(video.VideoError):
            video.validate_video_job(make_job(resolution="fullhd"))

    def test_rejects_mixed_reference_styles(self):
        with self.assertRaises(video.VideoError):
            video.validate_video_job(
                make_job(
                    image_urls=["https://assets.example.com/a.png"],
                    references=[
                        {
                            "type": "image",
                            "role": "reference",
                            "url": "https://assets.example.com/b.png",
                        }
                    ],
                )
            )

    def test_accepts_reference_object(self):
        job = make_job(
            references=[
                {
                    "type": "image",
                    "role": "first_frame",
                    "url": "https://assets.example.com/first.png",
                },
                {
                    "type": "image",
                    "role": "last_frame",
                    "url": "https://assets.example.com/last.png",
                },
            ]
        )
        video.validate_video_job(job)
        payload = video.build_video_payload(job)
        self.assertEqual(2, len(payload["references"]))
        self.assertNotIn("image_urls", payload)

    def test_payload_omits_empty_optional_fields(self):
        payload = video.build_video_payload(make_job(resolution=None, aspect_ratio=None))
        self.assertNotIn("resolution", payload)
        self.assertNotIn("aspect_ratio", payload)
        self.assertEqual(4, payload["seconds"])

    def test_duration_alias_validation(self):
        data = {"prompt": "x", "model": "minimax_h3", "seconds": 4, "duration": 5}
        with self.assertRaises(video.VideoError):
            video.merge_video_job(1, data, {}, {}, Path.cwd(), 300)


class VideoTaskTests(unittest.TestCase):
    def test_create_payload_uses_videos_endpoint(self):
        captured = {}

        def fake_request(path, key, payload=None, content_type="application/json", timeout=300):
            captured.update(
                {
                    "path": path,
                    "key": key,
                    "payload": json.loads(payload.decode("utf-8")),
                    "timeout": timeout,
                }
            )
            return {"id": "task-1", "status": "queued"}

        job = make_job()
        result = video.create_video_task(job, "secret-key", request_json_fn=fake_request)
        self.assertEqual("/videos", captured["path"])
        self.assertEqual("task-1", result["id"])
        self.assertEqual("minimax_h3", captured["payload"]["model"])

    def test_wait_for_video_completes(self):
        responses = iter(
            [
                {"id": "task-1", "status": "queued", "progress": 0},
                {"id": "task-1", "status": "in_progress", "progress": 40},
                {"id": "task-1", "status": "completed", "progress": 100},
            ]
        )
        sleeps = []

        def getter(task_id, key, timeout):
            return next(responses)

        final, history = video.wait_for_video(
            "task-1",
            "secret-key",
            poll_interval=0.01,
            poll_timeout=5,
            sleeper=sleeps.append,
            getter=getter,
        )
        self.assertEqual("completed", final["status"])
        self.assertEqual(["queued", "in_progress", "completed"], [item["status"] for item in history])
        self.assertEqual([0.01, 0.01], sleeps)

    def test_wait_for_video_stops_on_failure(self):
        def getter(task_id, key, timeout):
            return {"id": task_id, "status": "failed", "error": {"message": "upstream failed"}}

        with self.assertRaises(video.VideoTaskFailed) as raised:
            video.wait_for_video(
                "task-1",
                "secret-key",
                poll_timeout=5,
                sleeper=lambda _: None,
                getter=getter,
            )
        self.assertIn("task-1", str(raised.exception))
        self.assertIn("upstream failed", str(raised.exception))

    def test_wait_for_video_stops_on_unknown_status(self):
        with self.assertRaises(video.VideoError):
            video.wait_for_video(
                "task-1",
                "secret-key",
                poll_timeout=5,
                sleeper=lambda _: None,
                getter=lambda task_id, key, timeout: {"id": task_id, "status": "mystery"},
            )

    def test_wait_for_video_times_out_without_resubmit(self):
        with self.assertRaises(video.VideoTaskTimeout) as raised:
            video.wait_for_video(
                "task-1",
                "secret-key",
                poll_interval=10,
                poll_timeout=0,
                sleeper=lambda _: None,
                getter=lambda task_id, key, timeout: {"id": task_id, "status": "queued"},
            )
        self.assertIn("task-1", str(raised.exception))

    def test_task_path_escapes_id(self):
        paths = []

        def fake_request(path, key, payload=None, content_type="application/json", timeout=300):
            paths.append(path)
            return {"id": "task/one", "status": "queued"}

        video.get_video_task("task/one", "secret-key", request_json_fn=fake_request)
        self.assertEqual(["/videos/task%2Fone"], paths)

    def test_execute_video_job_persists_records(self):
        create_calls = []

        def fake_create(job, key):
            create_calls.append((job.id, key))
            return {"id": "task-1", "status": "queued"}

        def fake_wait(task_id, key, poll_interval, poll_timeout):
            return (
                {"id": task_id, "status": "completed", "usage": {"total_tokens": 12}},
                [{"id": task_id, "status": "queued"}, {"id": task_id, "status": "completed"}],
            )

        def fake_download(task_id, key, destination):
            destination = Path(destination)
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes(b"video")
            return {
                "path": str(destination),
                "bytes": 5,
                "sha256": "hash",
                "content_type": "video/mp4",
                "kind": "video",
            }

        with tempfile.TemporaryDirectory() as temp, patch.object(
            video,
            "media_tools_status",
            return_value={"available": True},
        ), patch.object(
            video,
            "verify_video_available",
            return_value={"visible": True, "warning": None},
        ):
            result = video.execute_video_job(
                make_job(),
                "secret-key",
                temp,
                create_fn=fake_create,
                wait_fn=fake_wait,
                download_fn=fake_download,
                probe_fn=lambda path: {
                    "duration": 4.0,
                    "width": 854,
                    "height": 480,
                },
            )
            result_path = Path(result["output_dir"]) / "job-result.json"
            task_path = Path(result["output_dir"]) / "task-final.json"
            self.assertTrue(result_path.is_file())
            self.assertTrue(task_path.is_file())
            self.assertEqual("success", result["status"])
            self.assertEqual("task-1", result["task_id"])
            self.assertEqual(1, len(create_calls))

    def test_query_failure_does_not_recreate_task(self):
        create_calls = []

        def fake_create(job, key):
            create_calls.append(job.id)
            return {"id": "task-keep", "status": "queued"}

        def fake_wait(*args, **kwargs):
            raise video.VideoTaskTimeout("task-keep timeout")

        with tempfile.TemporaryDirectory() as temp, patch.object(
            video,
            "verify_video_available",
            return_value={"visible": True, "warning": None},
        ):
            result = video.execute_video_job(
                make_job(),
                "secret-key",
                temp,
                create_fn=fake_create,
                wait_fn=fake_wait,
            )
        self.assertEqual("error", result["status"])
        self.assertEqual("task-keep", result["task_id"])
        self.assertEqual(1, len(create_calls))

    def test_model_visibility_is_advisory(self):
        def fake_request(path, key, timeout=300):
            return {"data": [{"id": "gpt-5"}]}

        visibility = video.verify_video_available(
            "minimax_h3",
            "secret-key",
            request_json_fn=fake_request,
        )
        self.assertFalse(visibility["visible"])
        self.assertIn("/models", visibility["warning"])


class VideoCliTests(unittest.TestCase):
    def test_dry_run_does_not_load_key(self):
        with patch.object(video, "load_key") as load_key:
            code = video.main(
                [
                    "run",
                    "--prompt",
                    "海边空镜",
                    "--model",
                    "minimax_h3",
                    "--seconds",
                    "4",
                    "--resolution",
                    "480p",
                    "--aspect-ratio",
                    "16:9",
                    "--out",
                    ".",
                    "--dry-run",
                ]
            )
        self.assertEqual(0, code)
        load_key.assert_not_called()

    def test_real_run_requires_confirm(self):
        code = video.main(
            [
                "run",
                "--prompt",
                "海边空镜",
                "--model",
                "minimax_h3",
                "--seconds",
                "4",
                "--resolution",
                "480p",
                "--aspect-ratio",
                "16:9",
                "--out",
                ".",
            ]
        )
        self.assertEqual(1, code)

    def test_get_command(self):
        with patch.object(
            video,
            "load_key",
            return_value=("secret-key", "test"),
        ), patch.object(
            video,
            "get_video_task",
            return_value={"id": "task-1", "status": "completed"},
        ):
            self.assertEqual(0, video.main(["get", "task-1"]))

    def test_wait_and_download_commands(self):
        final = {"id": "task-1", "status": "completed"}
        with patch.object(
            video,
            "load_key",
            return_value=("secret-key", "test"),
        ), patch.object(
            video,
            "wait_for_video",
            return_value=(final, [final]),
        ), patch.object(
            video,
            "download_video",
            return_value={"path": "output.mp4", "kind": "video"},
        ):
            self.assertEqual(
                0,
                video.main(
                    [
                        "wait",
                        "task-1",
                        "--download",
                        "--out",
                        "output.mp4",
                    ]
                ),
            )
        with patch.object(
            video,
            "load_key",
            return_value=("secret-key", "test"),
        ), patch.object(
            video,
            "download_video",
            return_value={"path": "output.mp4", "kind": "video"},
        ):
            self.assertEqual(
                0,
                video.main(["download", "task-1", "--out", "output.mp4"]),
            )

    def test_error_command_returns_one(self):
        with patch.object(
            video,
            "load_key",
            return_value=("secret-key", "test"),
        ), patch.object(
            video,
            "get_video_task",
            side_effect=video.VideoError("query failed"),
        ):
            self.assertEqual(1, video.main(["get", "task-1"]))


if __name__ == "__main__":
    unittest.main()
