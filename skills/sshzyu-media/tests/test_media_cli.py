from __future__ import annotations

from pathlib import Path
import sys
import unittest
from unittest.mock import patch


SKILL_ROOT = Path(__file__).resolve().parents[1]
SCRIPT_ROOT = SKILL_ROOT / "scripts"
sys.path.insert(0, str(SCRIPT_ROOT))

import sshzyu_media  # noqa: E402


class MediaCliTests(unittest.TestCase):
    def test_help(self):
        self.assertEqual(0, sshzyu_media.main(["--help"]))

    def test_no_args_and_version(self):
        self.assertEqual(2, sshzyu_media.main([]))
        self.assertEqual(0, sshzyu_media.main(["--version"]))

    def test_image_subcommand_dispatch(self):
        with patch.object(sshzyu_media, "image_main", return_value=7) as image_main:
            self.assertEqual(7, sshzyu_media.main(["image", "cat"]))
        image_main.assert_called_once_with(["cat"])

    def test_video_subcommand_dispatch(self):
        with patch.object(sshzyu_media, "video_main", return_value=8) as video_main:
            self.assertEqual(8, sshzyu_media.main(["video", "run"]))
        video_main.assert_called_once_with(["run"])

    def test_drama_subcommand_dispatch(self):
        with patch.object(sshzyu_media, "drama_main", return_value=9) as drama_main:
            self.assertEqual(9, sshzyu_media.main(["drama", "run"]))
        drama_main.assert_called_once_with(["run"])

    def test_legacy_image_flags_dispatch(self):
        argv = ["cat", "--model", "gpt-image-2.6-sunburst"]
        with patch.object(sshzyu_media, "image_main", return_value=3) as image_main:
            self.assertEqual(3, sshzyu_media.main(argv))
        image_main.assert_called_once_with(argv)

    def test_filter_models(self):
        payload = {
            "object": "list",
            "data": [
                {"id": "gpt-image-2.6-sunburst"},
                {"id": "minimax_h3"},
                {"id": "gpt-5"},
            ],
        }
        filtered = sshzyu_media.filter_models(payload, "video")
        self.assertEqual([{"id": "minimax_h3", "kind": "video"}], filtered["data"])
        self.assertEqual(1, filtered["filtered_count"])

    def test_models_main_uses_filtering(self):
        payload = {
            "object": "list",
            "data": [
                {"id": "gpt-image-2.6-sunburst"},
                {"id": "minimax_h3"},
            ],
        }
        with patch.object(
            sshzyu_media,
            "load_key",
            return_value=("secret-key", "test"),
        ), patch.object(
            sshzyu_media,
            "request_json",
            return_value=payload,
        ):
            code = sshzyu_media.models_main(["--kind", "image"])
        self.assertEqual(0, code)

    def test_doctor_main_does_not_require_network(self):
        with patch.object(sshzyu_media, "read_env_key", return_value=None), patch.object(
            sshzyu_media,
            "read_credential",
            return_value=None,
        ), patch.dict("os.environ", {"SSHZYU_API_KEY": "secret-key"}, clear=False):
            code = sshzyu_media.doctor_main([])
        self.assertEqual(0, code)


if __name__ == "__main__":
    unittest.main()
