#!/usr/bin/env python3
"""Compatibility bridge to the independent sshzyu-co-image skill."""

from __future__ import annotations

import importlib.util
import os
from pathlib import Path
import sys


def _candidate_roots():
    here = Path(__file__).resolve()
    yield here.parents[2] / "sshzyu-co-image"
    yield Path.home() / ".newmax" / "skills" / "sshzyu-co-image"
    custom = os.environ.get("NEWMAX_SKILLS_DIR")
    if custom:
        yield Path(custom).expanduser() / "sshzyu-co-image"


def _load_image_module():
    for root in _candidate_roots():
        module_path = root / "scripts" / "sshzyu_image.py"
        if not module_path.is_file() or module_path.resolve() == Path(__file__).resolve():
            continue
        script_dir = str(module_path.parent)
        if script_dir not in sys.path:
            sys.path.insert(0, script_dir)
        spec = importlib.util.spec_from_file_location("sshzyu_co_image_impl", module_path)
        if spec is None or spec.loader is None:
            continue
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module
    raise ImportError(
        "未找到独立生图技能 sshzyu-co-image，请先安装 "
        "skills/sshzyu-co-image。"
    )


_impl = _load_image_module()
for _name in dir(_impl):
    if not _name.startswith("__"):
        globals()[_name] = getattr(_impl, _name)

__all__ = [name for name in globals() if not name.startswith("_")]
