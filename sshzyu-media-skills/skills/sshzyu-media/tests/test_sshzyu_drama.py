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

import sshzyu_drama as drama  # noqa: E402


MANIFEST = {
    "project": "test-drama",
    "defaults": {
        "model": "minimax_h3",
        "seconds": 4,
        "resolution": "480p",
        "aspect_ratio": "9:16",
    },
    "shots": [
        {"id": "EP01_SH001", "prompt": "雨夜街头，女主回头"},
        {"id": "EP01_SH002", "prompt_file": "prompts/shot-02.txt", "seconds": 5},
    ],
    "assembly": {
        "resolution": "1080x1920",
        "fps": 25,
        "audio": "audio/voice.wav",
    },
}


class DramaManifestTests(unittest.TestCase):
    def write_manifest(self, root: Path, data):
        path = root / "drama.json"
        path.write_text(json.dumps(data, ensure_ascii=False), encoding="utf-8")
        return path

    def test_load_manifest_resolves_relative_paths(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "prompts").mkdir()
            (root / "prompts" / "shot-02.txt").write_text("第二镜头", encoding="utf-8")
            path = self.write_manifest(root, MANIFEST)
            loaded = drama.load_drama_manifest(path)
            jobs = drama.build_drama_jobs(loaded["data"], {}, loaded["path"].parent, 300)
        self.assertEqual(2, len(jobs))
        self.assertEqual("第二镜头", jobs[1].prompt)
        self.assertEqual(5, jobs[1].seconds)

    def test_duplicate_shot_ids_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            data = json.loads(json.dumps(MANIFEST))
            data["shots"][1]["prompt"] = "第二镜头"
            data["shots"][1].pop("prompt_file")
            data["shots"][1]["id"] = "EP01_SH001"
            path = self.write_manifest(root, data)
            loaded = drama.load_drama_manifest(path)
            with self.assertRaises(drama.DramaError):
                drama.build_drama_jobs(loaded["data"], {}, loaded["path"].parent, 300)

    def test_unknown_shot_field_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            data = json.loads(json.dumps(MANIFEST))
            data["shots"][0]["surprise"] = True
            path = self.write_manifest(root, data)
            loaded = drama.load_drama_manifest(path)
            with self.assertRaises(drama.DramaError):
                drama.build_drama_jobs(loaded["data"], {}, loaded["path"].parent, 300)

    def test_dry_run_does_not_load_key(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "prompts").mkdir()
            (root / "prompts" / "shot-02.txt").write_text("第二镜头", encoding="utf-8")
            path = self.write_manifest(root, MANIFEST)
            with patch.object(drama, "load_key") as load_key:
                code = drama.main(["run", str(path), "--out", str(root), "--dry-run"])
        self.assertEqual(0, code)
        load_key.assert_not_called()

    def test_run_and_resume_skip_completed_shots(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            data = {
                "project": "resume-test",
                "defaults": {
                    "model": "minimax_h3",
                    "seconds": 4,
                    "resolution": "480p",
                    "aspect_ratio": "9:16",
                },
                "shots": [
                    {"id": "EP01_SH001", "prompt": "镜头一"},
                    {"id": "EP01_SH002", "prompt": "镜头二"},
                ],
                "assembly": {"resolution": "1080x1920", "fps": 25},
            }
            path = self.write_manifest(root, data)
            calls = []

            def fake_execute(job, key, run_dir, verify_model=True):
                calls.append(job.id)
                shot_dir = Path(run_dir) / job.id
                shot_dir.mkdir(parents=True, exist_ok=True)
                video_path = shot_dir / "video.mp4"
                video_path.write_bytes(b"video")
                result = {
                    "id": job.id,
                    "status": "success",
                    "output_dir": str(shot_dir),
                    "video": {"path": str(video_path)},
                    "error": None,
                }
                (shot_dir / "job-result.json").write_text(
                    json.dumps(result),
                    encoding="utf-8",
                )
                return result

            code = drama.run_drama(
                path,
                root / "out",
                "secret-key",
                execute_fn=fake_execute,
            )
            self.assertEqual(0, code)
            self.assertEqual(["EP01_SH001", "EP01_SH002"], calls)
            run_dir = next((root / "out").glob("drama-*"))
            resume_code = drama.run_drama(
                path,
                root / "ignored",
                "secret-key",
                resume_run=run_dir,
                execute_fn=fake_execute,
            )
            self.assertEqual(0, resume_code)
            self.assertEqual(["EP01_SH001", "EP01_SH002"], calls)
            summary = json.loads((run_dir / "drama-summary.json").read_text(encoding="utf-8"))
            self.assertEqual(2, summary["shots_skipped"])

    def test_resolve_assembly_rejects_invalid_resolution(self):
        with self.assertRaises(drama.DramaError):
            drama.resolve_assembly(
                {"assembly": {"resolution": "vertical", "fps": 25}},
                Path.cwd(),
            )


class DramaAssemblyTests(unittest.TestCase):
    def test_build_assembly_command_normalizes_clips_and_audio(self):
        command = drama.build_assembly_command(
            video_paths=[Path("a.mp4"), Path("b.mp4")],
            output_path=Path("final.mp4"),
            resolution="1080x1920",
            fps=25,
            audio_path=Path("voice.wav"),
            total_duration=7.5,
        )
        joined = " ".join(command)
        self.assertIn("scale=1080:1920", joined)
        self.assertIn("pad=1080:1920", joined)
        self.assertIn("fps=25", joined)
        self.assertIn("concat=n=2:v=1:a=0", joined)
        self.assertIn("libx264", command)
        self.assertIn("+faststart", command)
        self.assertTrue(any(Path(item).name == "voice.wav" for item in command))
        self.assertIn("7.500000", command)

    def test_assemble_uses_runner_and_verifies_output(self):
        calls = []

        def runner(command, **kwargs):
            calls.append((command, kwargs))
            return type("Result", (), {"returncode": 0, "stdout": "", "stderr": ""})()

        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            inputs = []
            for index in range(2):
                path = root / f"shot-{index}.mp4"
                path.write_bytes(b"video")
                inputs.append(path)
            output = root / "final.mp4"
            with patch.object(drama, "probe_media", return_value={"duration": 2.5, "width": 1080, "height": 1920}):
                result = drama.assemble_videos(
                    inputs,
                    output,
                    resolution="1080x1920",
                    fps=25,
                    runner=runner,
                    allow_missing_output_for_test=True,
                )
        self.assertEqual(output, result["path"])
        self.assertEqual(1, len(calls))
        self.assertIn("concat=n=2:v=1:a=0", " ".join(calls[0][0]))

    def test_discover_outputs_skips_failed_shots(self):
        with tempfile.TemporaryDirectory() as temp:
            run_dir = Path(temp)
            for shot_id, status in (("EP01_SH001", "success"), ("EP01_SH002", "error")):
                shot_dir = run_dir / shot_id
                shot_dir.mkdir()
                video_path = shot_dir / "video.mp4"
                if status == "success":
                    video_path.write_bytes(b"video")
                (shot_dir / "job-result.json").write_text(
                    json.dumps(
                        {
                            "id": shot_id,
                            "status": status,
                            "video": {"path": str(video_path)} if status == "success" else None,
                        }
                    ),
                    encoding="utf-8",
                )
            outputs = drama.discover_completed_outputs(run_dir)
        self.assertEqual(["EP01_SH001"], [item["id"] for item in outputs])

    def test_assemble_main_uses_discovered_outputs(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            video_path = root / "shot.mp4"
            video_path.write_bytes(b"video")
            output = root / "final.mp4"
            with patch.object(
                drama,
                "discover_completed_outputs",
                return_value=[{"id": "EP01_SH001", "path": video_path}],
            ), patch.object(
                drama,
                "assemble_videos",
                return_value={
                    "path": output,
                    "width": 1080,
                    "height": 1920,
                    "duration": 4.0,
                },
            ) as assemble:
                code = drama.main(
                    [
                        "assemble",
                        str(root),
                        "--out",
                        str(output),
                        "--resolution",
                        "1080x1920",
                    ]
                )
        self.assertEqual(0, code)
        assemble.assert_called_once()

    def test_assemble_main_fails_without_outputs(self):
        with tempfile.TemporaryDirectory() as temp:
            with patch.object(drama, "discover_completed_outputs", return_value=[]):
                code = drama.main(
                    [
                        "assemble",
                        temp,
                        "--out",
                        str(Path(temp) / "final.mp4"),
                    ]
                )
        self.assertEqual(1, code)


if __name__ == "__main__":
    unittest.main()
