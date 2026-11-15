#!/usr/bin/env python3
"""Short-drama shot manifest runner and FFmpeg assembly for SSHZYU media."""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import sys
import uuid

sys.path.insert(0, str(Path(__file__).resolve().parent))

from media_common import (
    MediaError,
    read_json,
    redact_secret,
    run_checked,
    safe_filename,
    validate_shot_id,
    write_json,
)
from sshzyu_image import DEFAULT_ENV_FILE, load_key
from sshzyu_video import (
    VIDEO_JOB_KEYS,
    VideoError,
    execute_video_job,
    merge_video_job,
    print_video_plan,
    probe_media,
)


MAX_SHOTS = 64
MANIFEST_KEYS = {"project", "defaults", "shots", "assembly"}
ASSEMBLY_KEYS = {"resolution", "fps", "audio"}


class DramaError(MediaError):
    pass


class DramaAssemblyError(DramaError):
    pass


def emit(message, error=False):
    print(message, file=sys.stderr if error else sys.stdout, flush=True)


def load_drama_manifest(path):
    manifest_path = Path(path).expanduser().resolve()
    data = read_json(manifest_path)
    if not isinstance(data, dict):
        raise DramaError("短剧 manifest 顶层必须是对象。")
    unknown = set(data) - MANIFEST_KEYS
    if unknown:
        raise DramaError(f"manifest 包含未知字段: {', '.join(sorted(unknown))}")
    defaults = data.get("defaults", {})
    shots = data.get("shots")
    assembly = data.get("assembly", {})
    if not isinstance(defaults, dict):
        raise DramaError("manifest.defaults 必须是对象。")
    if not isinstance(shots, list) or not shots:
        raise DramaError("manifest.shots 必须是非空数组。")
    if len(shots) > MAX_SHOTS:
        raise DramaError(f"单次最多 {MAX_SHOTS} 个镜头。")
    if not isinstance(assembly, dict):
        raise DramaError("manifest.assembly 必须是对象。")
    unknown_assembly = set(assembly) - ASSEMBLY_KEYS
    if unknown_assembly:
        raise DramaError(
            f"manifest.assembly 包含未知字段: {', '.join(sorted(unknown_assembly))}"
        )
    return {
        "data": data,
        "path": manifest_path,
    }


def _manifest_overrides(overrides):
    allowed = {
        key: value
        for key, value in (overrides or {}).items()
        if value is not None and key in VIDEO_JOB_KEYS
    }
    return allowed


def build_drama_jobs(data, overrides, base_dir, timeout):
    defaults = data.get("defaults", {})
    shots = data.get("shots", [])
    jobs = []
    seen = set()
    normalized_overrides = _manifest_overrides(overrides)
    for index, raw in enumerate(shots, 1):
        if not isinstance(raw, dict):
            raise DramaError(f"镜头 {index} 必须是对象。")
        shot_id = validate_shot_id(raw.get("id", f"SHOT-{index:03d}"))
        if shot_id in seen:
            raise DramaError(f"镜头 id 重复: {shot_id}")
        seen.add(shot_id)
        try:
            job = merge_video_job(
                index,
                raw,
                defaults,
                normalized_overrides,
                base_dir,
                timeout,
            )
        except VideoError as exc:
            raise DramaError(f"{shot_id}: {exc}") from None
        job.id = shot_id
        jobs.append(job)
    return jobs


def resolve_assembly(data, base_dir):
    assembly = data.get("assembly", {})
    resolution = assembly.get("resolution", "1080x1920")
    if not re.fullmatch(r"[1-9]\d{2,4}x[1-9]\d{2,4}", str(resolution)):
        raise DramaError("assembly.resolution 必须是宽x高，例如 1080x1920。")
    try:
        fps = int(assembly.get("fps", 25))
    except (TypeError, ValueError):
        raise DramaError("assembly.fps 必须是整数。") from None
    if not 1 <= fps <= 120:
        raise DramaError("assembly.fps 必须在 1 到 120 之间。")
    audio = assembly.get("audio")
    if audio:
        audio = str(Path(base_dir, audio).expanduser().resolve())
    return {
        "resolution": str(resolution),
        "fps": fps,
        "audio": audio,
    }


def print_drama_plan(jobs, assembly):
    total_seconds = sum(job.seconds for job in jobs)
    emit(
        f"DRAMA PLAN: shots={len(jobs)}; generated_seconds={total_seconds}; "
        f"output={assembly['resolution']}@{assembly['fps']}fps; "
        f"audio={'external' if assembly['audio'] else 'none'}"
    )
    for index, job in enumerate(jobs, 1):
        print_video_plan(job)
        emit(f"SHOT {index:02d}/{len(jobs):02d}: {job.id}")
    emit("COST WARNING: 每个镜头都会创建独立付费视频任务；默认顺序执行，不自动重试。")


def _new_drama_dir(out_dir, project):
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
    path = (
        Path(out_dir).expanduser().resolve()
        / f"drama-{safe_filename(project, 'project')}-{stamp}-{uuid.uuid4().hex[:8]}"
    )
    path.mkdir(parents=True, exist_ok=False)
    return path


def _find_shot_result(run_dir, shot_id):
    for result_path in Path(run_dir).glob("*/job-result.json"):
        try:
            result = read_json(result_path)
        except MediaError:
            continue
        if result.get("id") == shot_id:
            return result_path, result
    return None, None


def _write_state(run_dir, project, entries):
    state = {
        "project": project,
        "updated_at": datetime.now(timezone.utc).isoformat(),
        "shots": entries,
    }
    write_json(Path(run_dir) / "drama-state.json", state)
    return state


def run_drama(
    manifest_path,
    out_dir,
    key=None,
    *,
    overrides=None,
    resume_run=None,
    continue_on_error=False,
    execute_fn=None,
    verify_model=True,
):
    loaded = load_drama_manifest(manifest_path)
    data = loaded["data"]
    jobs = build_drama_jobs(
        data,
        overrides or {},
        loaded["path"].parent,
        float((overrides or {}).get("timeout", 600)),
    )
    assembly = resolve_assembly(data, loaded["path"].parent)
    snapshot_data = json.loads(json.dumps(data, ensure_ascii=False))
    if assembly["audio"]:
        snapshot_data.setdefault("assembly", {})["audio"] = assembly["audio"]
    if resume_run:
        run_dir = Path(resume_run).expanduser().resolve()
        if not run_dir.is_dir():
            raise DramaError(f"resume 目录不存在: {run_dir}")
        snapshot = run_dir / "manifest.snapshot.json"
        if not snapshot.is_file():
            write_json(snapshot, snapshot_data)
    else:
        run_dir = _new_drama_dir(out_dir, data.get("project", "project"))
        write_json(run_dir / "manifest.snapshot.json", snapshot_data)
    execute_fn = execute_fn or execute_video_job
    entries = []
    failed = 0
    partial = 0
    skipped = 0
    for index, job in enumerate(jobs, 1):
        result_path, prior = _find_shot_result(run_dir, job.id)
        if (
            prior
            and prior.get("status") in {"success", "partial"}
            and prior.get("video", {}).get("path")
            and Path(prior["video"]["path"]).is_file()
        ):
            emit(f"SKIP: {job.id}; existing status={prior.get('status')}")
            entries.append(
                {
                    "id": job.id,
                    "status": prior.get("status"),
                    "result_path": str(result_path),
                    "video_path": prior["video"]["path"],
                }
            )
            skipped += 1
            continue
        emit(f"DRAMA SHOT {index}/{len(jobs)}: {job.id}")
        result = execute_fn(
            job,
            key,
            run_dir,
            verify_model=verify_model if index == 1 else False,
        )
        entry = {
            "id": job.id,
            "status": result.get("status"),
            "result_path": str(Path(result.get("output_dir", "")) / "job-result.json"),
            "video_path": (result.get("video") or {}).get("path"),
            "error": result.get("error"),
        }
        entries.append(entry)
        _write_state(run_dir, data.get("project", "project"), entries)
        if result.get("status") == "partial":
            partial += 1
        if result.get("status") == "error":
            failed += 1
            if not continue_on_error:
                emit(f"STOP: {job.id} failed; later shots were not submitted.", error=True)
                break
    state = _write_state(run_dir, data.get("project", "project"), entries)
    summary = {
        "run_dir": str(run_dir),
        "project": data.get("project", "project"),
        "shots_requested": len(jobs),
        "shots_completed": sum(
            entry["status"] in {"success", "partial"} for entry in entries
        ),
        "shots_skipped": skipped,
        "shots_partial": partial,
        "shots_failed": failed,
        "assembly": assembly,
        "state": state,
    }
    write_json(run_dir / "drama-summary.json", summary)
    emit(
        f"DRAMA SUMMARY: completed={summary['shots_completed']}/{len(jobs)}; "
        f"partial={partial}; failed={failed}; skipped={skipped}"
    )
    emit(f"RUN: {run_dir}")
    if failed:
        return 1
    if partial or summary["shots_completed"] < len(jobs):
        return 2
    return 0


def discover_completed_outputs(run_dir):
    directory = Path(run_dir).expanduser().resolve()
    if not directory.is_dir():
        raise DramaError(f"运行目录不存在: {directory}")
    snapshot_path = directory / "manifest.snapshot.json"
    shot_order = []
    if snapshot_path.is_file():
        snapshot = read_json(snapshot_path)
        shot_order = [
            str(shot.get("id"))
            for shot in snapshot.get("shots", [])
            if isinstance(shot, dict) and shot.get("id")
        ]
    by_id = {}
    for result_path in directory.glob("*/job-result.json"):
        try:
            result = read_json(result_path)
        except MediaError:
            continue
        if result.get("status") not in {"success", "partial"}:
            continue
        video_path = (result.get("video") or {}).get("path")
        if not video_path or not Path(video_path).is_file():
            continue
        by_id[str(result.get("id"))] = {
            "id": result.get("id"),
            "status": result.get("status"),
            "path": Path(video_path),
            "result_path": result_path,
        }
    if shot_order:
        ordered = [by_id[shot_id] for shot_id in shot_order if shot_id in by_id]
    else:
        ordered = sorted(by_id.values(), key=lambda item: str(item["id"]))
    return ordered


def _parse_resolution(value):
    match = re.fullmatch(r"([1-9]\d{2,4})x([1-9]\d{2,4})", str(value))
    if not match:
        raise DramaAssemblyError("输出分辨率必须是宽x高。")
    return int(match.group(1)), int(match.group(2))


def build_assembly_command(
    video_paths,
    output_path,
    *,
    resolution,
    fps,
    audio_path=None,
    total_duration=None,
):
    if not video_paths:
        raise DramaAssemblyError("没有可拼接的视频。")
    width, height = _parse_resolution(resolution)
    if not 1 <= int(fps) <= 120:
        raise DramaAssemblyError("fps 无效。")
    command = ["ffmpeg", "-hide_banner", "-nostdin", "-n"]
    for path in video_paths:
        command.extend(["-i", str(Path(path).expanduser().resolve())])
    if audio_path:
        command.extend(["-i", str(Path(audio_path).expanduser().resolve())])
    filters = []
    concat_inputs = []
    for index in range(len(video_paths)):
        filters.append(
            f"[{index}:v]"
            f"scale={width}:{height}:force_original_aspect_ratio=decrease,"
            f"pad={width}:{height}:(ow-iw)/2:(oh-ih)/2:color=black,"
            f"setsar=1,fps={int(fps)},format=yuv420p[v{index}]"
        )
        concat_inputs.append(f"[v{index}]")
    filters.append(
        "".join(concat_inputs) + f"concat=n={len(video_paths)}:v=1:a=0[outv]"
    )
    if audio_path:
        if total_duration is None or total_duration <= 0:
            raise DramaAssemblyError("附加音频时必须提供总时长。")
        audio_index = len(video_paths)
        filters.append(
            f"[{audio_index}:a]aresample=48000,"
            f"apad=whole_dur={float(total_duration):.6f}[outa]"
        )
    command.extend(["-filter_complex", ";".join(filters), "-map", "[outv]"])
    if audio_path:
        command.extend(["-map", "[outa]", "-t", f"{float(total_duration):.6f}"])
    command.extend(
        [
            "-c:v",
            "libx264",
            "-preset",
            "medium",
            "-crf",
            "18",
            "-pix_fmt",
            "yuv420p",
            "-movflags",
            "+faststart",
        ]
    )
    if audio_path:
        command.extend(["-c:a", "aac", "-b:a", "192k"])
    else:
        command.append("-an")
    command.append(str(Path(output_path).expanduser().resolve()))
    return command


def assemble_videos(
    video_paths,
    output_path,
    *,
    resolution="1080x1920",
    fps=25,
    audio_path=None,
    runner=None,
    probe_fn=None,
    allow_missing_output_for_test=False,
):
    paths = [Path(path).expanduser().resolve() for path in video_paths]
    for path in paths:
        if not path.is_file():
            raise DramaAssemblyError(f"输入视频不存在: {path}")
    output = Path(output_path).expanduser().resolve()
    if output.exists():
        raise DramaAssemblyError(f"输出文件已存在，不覆盖: {output}")
    if audio_path:
        audio_path = Path(audio_path).expanduser().resolve()
        if not audio_path.is_file():
            raise DramaAssemblyError(f"音频文件不存在: {audio_path}")
    runner = runner or run_checked
    probe_fn = probe_fn or probe_media
    durations = []
    for path in paths:
        info = probe_fn(path)
        duration = float(info.get("duration") or 0)
        if duration <= 0:
            raise DramaAssemblyError(f"无法读取视频时长: {path}")
        durations.append(duration)
    total_duration = sum(durations)
    output.parent.mkdir(parents=True, exist_ok=True)
    command = build_assembly_command(
        paths,
        output,
        resolution=resolution,
        fps=fps,
        audio_path=audio_path,
        total_duration=total_duration,
    )
    runner(command, timeout=7200)
    if not output.exists():
        if allow_missing_output_for_test:
            return {
                "path": output,
                "duration": total_duration,
                "width": _parse_resolution(resolution)[0],
                "height": _parse_resolution(resolution)[1],
                "verified": False,
            }
        raise DramaAssemblyError("ffmpeg 未生成输出文件。")
    verified = probe_fn(output)
    expected_width, expected_height = _parse_resolution(resolution)
    if (
        verified.get("width") != expected_width
        or verified.get("height") != expected_height
    ):
        raise DramaAssemblyError("输出视频分辨率与请求不一致。")
    return {
        "path": output,
        "duration": verified.get("duration", total_duration),
        "width": verified.get("width"),
        "height": verified.get("height"),
        "verified": True,
    }


def parser():
    p = argparse.ArgumentParser(
        prog="sshzyu_media.py drama",
        description="AI 漫剧/短剧镜头清单执行与 FFmpeg 成片装配。",
    )
    sub = p.add_subparsers(dest="command", required=True)
    run = sub.add_parser("run", help="按镜头清单顺序生成视频")
    run.add_argument("manifest")
    run.add_argument("--out", default="./sshzyu-drama")
    run.add_argument("--resume-run")
    run.add_argument("--continue-on-error", action="store_true")
    run.add_argument("--model")
    run.add_argument("--seconds", type=int)
    run.add_argument("--resolution")
    run.add_argument("--aspect-ratio")
    run.add_argument("--timeout", type=float, default=600)
    run.add_argument("--poll-interval", type=float)
    run.add_argument("--poll-timeout", type=float)
    run.add_argument("--env-file", default=str(DEFAULT_ENV_FILE))
    run.add_argument("--dry-run", action="store_true")
    run.add_argument("--confirm", action="store_true")
    run.add_argument("--no-verify-model", action="store_true")
    assembly = sub.add_parser("assemble", help="拼接已完成的镜头")
    assembly.add_argument("run_dir")
    assembly.add_argument("--out", required=True, help="输出 MP4 路径")
    assembly.add_argument("--resolution")
    assembly.add_argument("--fps", type=int)
    assembly.add_argument("--audio")
    return p


def main(argv=None):
    args = parser().parse_args(argv)
    key = None
    try:
        if args.command == "run":
            loaded = load_drama_manifest(args.manifest)
            data = loaded["data"]
            overrides = {
                "model": args.model,
                "seconds": args.seconds,
                "resolution": args.resolution,
                "aspect_ratio": args.aspect_ratio,
                "timeout": args.timeout,
                "poll_interval": args.poll_interval,
                "poll_timeout": args.poll_timeout,
            }
            jobs = build_drama_jobs(
                data,
                overrides,
                loaded["path"].parent,
                args.timeout,
            )
            assembly = resolve_assembly(data, loaded["path"].parent)
            print_drama_plan(jobs, assembly)
            if args.dry_run:
                return 0
            if not args.confirm:
                raise DramaError(
                    "整条短剧清单会产生多笔视频费用；确认后加 --confirm。"
                )
            key, source = load_key(args.env_file)
            emit(f"CREDENTIAL: {source}; sequential execution; no automatic retry")
            return run_drama(
                args.manifest,
                args.out,
                key,
                overrides=overrides,
                resume_run=args.resume_run,
                continue_on_error=args.continue_on_error,
                verify_model=not args.no_verify_model,
            )
        if args.command == "assemble":
            run_dir = Path(args.run_dir).expanduser().resolve()
            outputs = discover_completed_outputs(run_dir)
            if not outputs:
                raise DramaAssemblyError("没有找到已完成的镜头视频。")
            snapshot = run_dir / "manifest.snapshot.json"
            assembly = {"resolution": "1080x1920", "fps": 25, "audio": None}
            if snapshot.is_file():
                loaded = {"data": read_json(snapshot), "path": snapshot}
                assembly = resolve_assembly(loaded["data"], snapshot.parent)
            resolution = args.resolution or assembly["resolution"]
            fps = args.fps or assembly["fps"]
            audio = args.audio or assembly["audio"]
            result = assemble_videos(
                [item["path"] for item in outputs],
                args.out,
                resolution=resolution,
                fps=fps,
                audio_path=audio,
            )
            emit(
                f"ASSEMBLED: {result['path']} "
                f"({result['width']}x{result['height']}, {result['duration']:.2f}s)"
            )
            return 0
        raise DramaError(f"未知命令: {args.command}")
    except (MediaError, OSError, UnicodeError) as exc:
        emit("ERROR: " + redact_secret(exc, key), error=True)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
