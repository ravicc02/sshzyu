#!/usr/bin/env python3
"""Compatibility entry point. Prefer scripts/sshzyu_image.py."""

from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))

from scripts.sshzyu_image import main


if __name__ == "__main__":
    raise SystemExit(main())
