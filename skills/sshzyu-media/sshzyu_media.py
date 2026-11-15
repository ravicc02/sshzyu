#!/usr/bin/env python3
"""Primary entry point for the SSHZYU media skill."""

from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))

from scripts.sshzyu_media import main


if __name__ == "__main__":
    raise SystemExit(main())
