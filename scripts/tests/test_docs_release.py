"""Regression checks for complete, verifiable standalone docs deployments."""

from contextlib import contextmanager, redirect_stdout
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import threading
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).resolve().parents[1] / "docs-release.py"
spec = importlib.util.spec_from_file_location("docs_release", SCRIPT)
docs = importlib.util.module_from_spec(spec)
spec.loader.exec_module(docs)


class QuietHandler(SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass


@contextmanager
def site(directory):
    server = ThreadingHTTPServer(("127.0.0.1", 0), partial(QuietHandler, directory=str(directory)))
    worker = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.02}, daemon=True)
    worker.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}/"
    finally:
        server.shutdown()
        server.server_close()
        worker.join()


class DocsReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "repo"
        self.root.mkdir()
        for name in docs.REQUIRED:
            path = self.root / "docs" / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("test docs\n", encoding="utf-8")
        for name in ("index.html", "image/index.html", "vedio/index.html"):
            (self.root / "docs" / name).write_text(
                '<link href="/docs/docs-category.css"><a href="/docs/image/">Image</a>'
                '<script>fetch("content.md")</script>', encoding="utf-8"
            )
        self.run_git("init", "-q")
        self.run_git("add", "docs")
        self.run_git("-c", "user.name=Docs Tests", "-c", "user.email=docs-tests@example.invalid", "commit", "-qm", "docs fixture")
        self.root_patch = patch.object(docs, "ROOT", self.root)
        self.root_patch.start()
        self.addCleanup(self.root_patch.stop)

    def run_git(self, *args):
        subprocess.run(["git", "-C", str(self.root), *args], check=True, capture_output=True)

    def stage(self):
        with redirect_stdout(io.StringIO()):
            return docs.stage(Path(self.temp.name) / "stage")

    def test_missing_video_page_blocks_release(self):
        names = docs.tracked_docs()
        (self.root / "docs/vedio/index.html").unlink()
        with self.assertRaisesRegex(ValueError, "Not a regular"):
            docs.check(self.root / "docs", names)

    def test_reference_to_untracked_asset_is_rejected_even_if_present(self):
        (self.root / "docs/untracked.png").write_bytes(b"local-only")
        (self.root / "docs/content.md").write_text("![image](untracked.png)", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "untracked reference"):
            docs.check(self.root / "docs", docs.tracked_docs())

    def test_encoded_reference_cannot_escape_document_root(self):
        (self.root / "docs/content.md").write_text("[file](%2e%2e/outside.md)", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "escapes docs"):
            docs.check(self.root / "docs", docs.tracked_docs())

    def test_fetched_markdown_is_checked(self):
        (self.root / "docs/image/content.md").write_text("![missing](../iamge/missing.png)", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "untracked reference"):
            docs.check(self.root / "docs", docs.tracked_docs())

    def test_stage_excludes_untracked_local_files(self):
        (self.root / "docs/.env").write_text("local-only-placeholder", encoding="utf-8")
        bundle = self.stage()
        self.assertFalse((bundle / "docs/.env").exists())
        manifest = docs.load_manifest(bundle)
        self.assertEqual(len(manifest["files"]), len(docs.REQUIRED))
        self.assertTrue(manifest["docs_tree"])

    def test_uncommitted_docs_block_staging(self):
        (self.root / "docs/content.md").write_text("uncommitted", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "Commit docs"):
            self.stage()

    def test_http_verification_checks_files_and_directory_entries(self):
        bundle = self.stage()
        with site(bundle / "docs") as base, redirect_stdout(io.StringIO()):
            result = docs.verify(bundle, base)
        self.assertEqual(result["files_verified"], len(docs.REQUIRED))
        self.assertTrue((bundle / "verification.json").is_file())

    def test_http_200_with_old_content_is_not_success(self):
        bundle = self.stage()
        (bundle / "docs/image/content.md").write_text("old production content", encoding="utf-8")
        with site(bundle / "docs") as base, redirect_stdout(io.StringIO()):
            with self.assertRaisesRegex(ValueError, "HTTP content differs"):
                docs.verify(bundle, base)
        self.assertFalse((bundle / "verification.json").exists())

    def test_manifest_cannot_request_a_path_outside_docs(self):
        bundle = self.stage()
        path = bundle / "deployment.json"
        manifest = json.loads(path.read_text(encoding="utf-8"))
        manifest["files"][0]["path"] = "../outside.md"
        path.write_text(json.dumps(manifest), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "Invalid documentation manifest"):
            docs.load_manifest(bundle)


if __name__ == "__main__":
    unittest.main()
