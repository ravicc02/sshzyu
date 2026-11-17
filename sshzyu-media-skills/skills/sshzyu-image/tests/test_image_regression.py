from __future__ import annotations

from pathlib import Path
import sys
import unittest


SKILL_ROOT = Path(__file__).resolve().parents[1]
SCRIPT_ROOT = SKILL_ROOT / "scripts"
sys.path.insert(0, str(SCRIPT_ROOT))

import sshzyu_image  # noqa: E402


class ImageRegressionTests(unittest.TestCase):
    def test_default_model_is_available_submodel(self):
        self.assertEqual("gpt-image-2.6-sunburst", sshzyu_image.DEFAULT_MODEL)

    def test_submodel_capabilities_follow_26_family(self):
        self.assertTrue(sshzyu_image.model_capabilities("gpt-image-2.6-flare")["gif"])
        self.assertTrue(sshzyu_image.model_capabilities("gpt-image-2.6-sunburst")["svg"])

    def test_allowed_api_paths(self):
        allowed = [
            "/models",
            "/images/generations",
            "/images/edits",
            "/videos",
            "/videos/generations",
            "/videos/task_123",
            "/videos/task%2Fone",
            "/videos/task_123/content",
        ]
        for path in allowed:
            with self.subTest(path=path):
                self.assertTrue(sshzyu_image.allowed_api_path(path))
        for path in ("/v1/files", "/videos/task/content/extra", "/admin"):
            with self.subTest(path=path):
                self.assertFalse(sshzyu_image.allowed_api_path(path))


if __name__ == "__main__":
    unittest.main()
