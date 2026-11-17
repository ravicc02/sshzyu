from __future__ import annotations

from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import sys
import tempfile
import threading
import unittest
import urllib.request
from unittest.mock import patch


SKILL_ROOT = Path(__file__).resolve().parents[1]
SCRIPT_ROOT = SKILL_ROOT / "scripts"
sys.path.insert(0, str(SCRIPT_ROOT))

from media_common import (  # noqa: E402
    MediaError,
    SafeRedirectHandler,
    classify_model,
    detect_video_container,
    download_file,
    is_video_payload,
    make_temp_file,
    probe_media,
    read_json,
    redact_secret,
    safe_filename,
    run_checked,
    validate_reference_url,
    write_json,
)


MP4_BYTES = b"\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom" + b"\x00" * 32
WEBM_BYTES = b"\x1aE\xdf\xa3" + b"\x00" * 64


@contextmanager
def local_http_server(handler):
    server = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


class MediaCommonTests(unittest.TestCase):
    def test_classify_known_models(self):
        self.assertEqual("image", classify_model("gpt-image-2.6-sunburst"))
        self.assertEqual("video", classify_model("minimax_h3"))
        self.assertEqual("video", classify_model("sd-2.5-满血"))
        self.assertEqual("video", classify_model("videopipe-doubao-2.0"))
        self.assertEqual("other", classify_model("gpt-5"))

    def test_reference_url_validation(self):
        self.assertEqual(
            "https://assets.example.com/ref.png",
            validate_reference_url("https://assets.example.com/ref.png"),
        )
        invalid = [
            "http://assets.example.com/ref.png",
            "https://localhost/ref.png",
            "https://127.0.0.1/ref.png",
            "https://10.0.0.2/ref.png",
            "https://user:pass@assets.example.com/ref.png",
            "/tmp/ref.png",
        ]
        for value in invalid:
            with self.subTest(value=value):
                with self.assertRaises(MediaError):
                    validate_reference_url(value)

    def test_video_magic_validation(self):
        self.assertTrue(is_video_payload(MP4_BYTES))
        self.assertTrue(is_video_payload(WEBM_BYTES))
        self.assertFalse(is_video_payload(b"<!doctype html><html></html>"))
        self.assertFalse(is_video_payload(b"not a video"))
        self.assertEqual("mp4", detect_video_container(MP4_BYTES))
        self.assertEqual("webm", detect_video_container(WEBM_BYTES))

    def test_safe_filename(self):
        self.assertEqual("shot-01_final", safe_filename("../shot 01/final"))
        self.assertEqual("job", safe_filename("///"))

    def test_redaction_does_not_rewrite_task_ids(self):
        text = redact_secret("task-keep failed; Bearer abc123", "actual-secret")
        self.assertEqual("task-keep failed; Bearer [REDACTED]", text)

    def test_redirect_rejects_public_to_private(self):
        handler = SafeRedirectHandler()
        request = urllib.request.Request(
            "https://sshzyu.com/v1/videos/task/content"
        )
        with self.assertRaises(MediaError):
            handler.redirect_request(
                request,
                None,
                307,
                "Temporary Redirect",
                {},
                "https://127.0.0.1/video.mp4",
            )

    def test_json_round_trip_and_invalid_json(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "nested" / "record.json"
            write_json(path, {"ok": True, "中文": "值"})
            self.assertEqual({"ok": True, "中文": "值"}, read_json(path))
            path.write_text("{broken", encoding="utf-8")
            with self.assertRaises(MediaError):
                read_json(path)

    def test_probe_media_parses_ffprobe_json(self):
        payload = {
            "streams": [
                {
                    "index": 0,
                    "codec_type": "video",
                    "codec_name": "h264",
                    "width": 1080,
                    "height": 1920,
                    "r_frame_rate": "25/1",
                }
            ],
            "format": {
                "duration": "4.25",
                "format_name": "mov,mp4",
                "size": "1234",
            },
        }
        result = type(
            "Result",
            (),
            {
                "returncode": 0,
                "stdout": json.dumps(payload),
                "stderr": "",
            },
        )()
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "video.mp4"
            path.write_bytes(MP4_BYTES)
            with patch("media_common._tool_path", return_value="ffprobe"), patch(
                "media_common.subprocess.run",
                return_value=result,
            ):
                info = probe_media(path)
        self.assertEqual(1080, info["width"])
        self.assertEqual(1920, info["height"])
        self.assertEqual(4.25, info["duration"])
        self.assertFalse(info["has_audio"])

    def test_probe_media_rejects_missing_video_stream(self):
        result = type(
            "Result",
            (),
            {
                "returncode": 0,
                "stdout": '{"streams": [], "format": {"duration": "1"}}',
                "stderr": "",
            },
        )()
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "video.mp4"
            path.write_bytes(MP4_BYTES)
            with patch("media_common._tool_path", return_value="ffprobe"), patch(
                "media_common.subprocess.run",
                return_value=result,
            ):
                with self.assertRaises(MediaError):
                    probe_media(path)

    def test_run_checked_and_make_temp_file(self):
        result = type("Result", (), {"returncode": 0, "stdout": "", "stderr": ""})()
        with patch("media_common.subprocess.run", return_value=result):
            self.assertIs(result, run_checked(["ffmpeg", "-version"], timeout=1))
        with tempfile.TemporaryDirectory() as temp:
            path = make_temp_file(temp, ".txt")
            self.assertTrue(path.is_file())
            path.unlink()

    def test_download_rejects_html_payload(self):
        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                body = b"<!doctype html><title>login</title>"
                self.send_response(200)
                self.send_header("Content-Type", "text/html")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *args):
                return

        with local_http_server(Handler) as base, tempfile.TemporaryDirectory() as temp:
            destination = Path(temp) / "video.mp4"
            with self.assertRaises(MediaError):
                download_file(base + "/content", destination)
            self.assertFalse(destination.exists())
            self.assertEqual([], list(Path(temp).glob("*.part-*")))

    def test_download_redirect_drops_authorization_across_origins(self):
        captured = []

        class TargetHandler(BaseHTTPRequestHandler):
            def do_GET(self):
                captured.append(("target", self.headers.get("Authorization")))
                self.send_response(200)
                self.send_header("Content-Type", "video/mp4")
                self.send_header("Content-Length", str(len(MP4_BYTES)))
                self.end_headers()
                self.wfile.write(MP4_BYTES)

            def log_message(self, *args):
                return

        with local_http_server(TargetHandler) as target_base:
            class RedirectHandler(BaseHTTPRequestHandler):
                def do_GET(self):
                    captured.append(("redirect", self.headers.get("Authorization")))
                    self.send_response(307)
                    self.send_header("Location", target_base + "/signed/video.mp4")
                    self.end_headers()

                def log_message(self, *args):
                    return

            with local_http_server(RedirectHandler) as redirect_base:
                with tempfile.TemporaryDirectory() as temp:
                    destination = Path(temp) / "video.mp4"
                    record = download_file(
                        redirect_base + "/v1/videos/task/content",
                        destination,
                        headers={"Authorization": "Bearer secret-value"},
                    )
                    self.assertTrue(destination.exists())
                    self.assertEqual("video", record["kind"])

        self.assertEqual("Bearer secret-value", captured[0][1])
        self.assertIsNone(captured[1][1])

    def test_download_enforces_size_limit(self):
        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                body = MP4_BYTES + b"x" * 1024
                self.send_response(200)
                self.send_header("Content-Type", "video/mp4")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *args):
                return

        with local_http_server(Handler) as base, tempfile.TemporaryDirectory() as temp:
            destination = Path(temp) / "video.mp4"
            with self.assertRaises(MediaError):
                download_file(base + "/content", destination, max_bytes=64)
            self.assertFalse(destination.exists())


if __name__ == "__main__":
    unittest.main()
