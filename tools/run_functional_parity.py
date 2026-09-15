#!/usr/bin/env python3
"""Run deterministic functional comparisons; never report benchmark timings."""

from __future__ import annotations

import argparse
import ctypes
import hashlib
import json
import os
import platform
import subprocess
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
RUST_MANIFEST = ROOT / "reference" / "rust-driver" / "Cargo.toml"
GO_COMPAT = ROOT / "bench" / "go-compat"
OPERATIONS = ("raw_walk_serial", "raw_walk_sorted", "scan_paths", "scan", "scan_compact")


def run(command: list[str], cwd: Path = ROOT, input_text: str | None = None) -> str:
    result = subprocess.run(
        command,
        cwd=cwd,
        input=input_text,
        text=True,
        capture_output=True,
        check=False,
        timeout=600,
    )
    if result.returncode:
        raise RuntimeError(
            f"{' '.join(command)} failed ({result.returncode})\n"
            f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}"
        )
    return result.stdout


def write_fixture(root: Path) -> tuple[bool, str | None]:
    files = {
        ".gitignore": "*.tmp\nignored-dir/\n",
        ".ignore": "*.log\n",
        ".weavatrixignore": "*.secret\n",
        "a.txt": "alpha\n",
        ".hidden.txt": "dot names are selected by default\n",
        "drop.tmp": "ignored by gitignore\n",
        "drop.log": "ignored by dot-ignore\n",
        "drop.secret": "ignored by custom ignore\n",
        "ignored-dir/inside.txt": "ignored directory\n",
        "node_modules/package.js": "standard skip\n",
        "sub/b.go": "package b\n",
        "sub/empty.txt": "",
        "unicode/дані.txt": "дані\n",
    }
    for relative, content in files.items():
        path = root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8", newline="\n")
    (root / "binary.bin").write_bytes(b"text\x00binary")
    return add_symlink_fixture(root)


def add_symlink_fixture(root: Path) -> tuple[bool, str | None]:
    links = root / "links"
    links.mkdir()
    try:
        os.symlink("../sub", links / "internal-dir", target_is_directory=True)
    except OSError as error:
        links.rmdir()
        code = getattr(error, "winerror", None) or error.errno
        return False, f"error {code}: {error.strerror}"
    outside = root.parent / "outside"
    outside.mkdir()
    (outside / "outside.txt").write_text("outside\n", encoding="utf-8", newline="\n")
    os.symlink("../a.txt", links / "file-link")
    os.symlink("../../outside", links / "escape", target_is_directory=True)
    os.symlink("..", root / "sub" / "back", target_is_directory=True)
    return True, None


def fixture_fingerprint(root: Path) -> str:
    digest = hashlib.sha256()
    for path in sorted(root.rglob("*")):
        if not path.is_symlink() and not path.is_file():
            continue
        digest.update(path.relative_to(root).as_posix().encode())
        digest.update(b"\0")
        if path.is_symlink():
            digest.update(b"link\0")
            digest.update(os.readlink(path).encode())
        else:
            digest.update(path.read_bytes())
        digest.update(b"\xff")
    return "sha256:" + digest.hexdigest()


def build_drivers(output: Path) -> tuple[Path, Path]:
    go_binary = output / ("treestamp-driver.exe" if os.name == "nt" else "treestamp-driver")
    run(["go", "build", "-o", str(go_binary), "./cmd/treestamp-driver"])
    run(["cargo", "build", "--release", "--manifest-path", str(RUST_MANIFEST)])
    metadata = json.loads(
        run(
            [
                "cargo",
                "metadata",
                "--format-version",
                "1",
                "--no-deps",
                "--manifest-path",
                str(RUST_MANIFEST),
            ]
        )
    )
    rust_name = "treestamp-reference-driver.exe" if os.name == "nt" else "treestamp-reference-driver"
    rust_binary = Path(metadata["target_directory"]) / "release" / rust_name
    if not rust_binary.is_file():
        raise RuntimeError(f"Rust driver was not built: {rust_binary}")
    return go_binary, rust_binary


def driver_payload(
    binary: Path,
    operation: str,
    root: Path,
    options: dict[str, Any] | None = None,
) -> dict[str, Any]:
    request_data: dict[str, Any] = {"op": operation, "root": str(root)}
    if options:
        request_data["options"] = options
    request = json.dumps(request_data, ensure_ascii=False)
    output = run([str(binary)], input_text=request)
    payload = json.loads(output)
    if payload.get("error"):
        raise RuntimeError(f"{binary.name} {operation}: {payload['error']}")
    return payload


def normalize(payload: dict[str, Any], operation: str) -> Any:
    data = payload.get("data") or {}
    if operation.startswith("raw_walk"):
        entries = [normalize_paths(dict(item)) for item in data.get("entries") or []]
        if operation == "raw_walk_serial":
            entries.sort(key=canonical)
        return entries
    if operation == "scan_paths":
        return [slash(path) for path in data.get("paths") or []]
    out = dict(data)
    for field in ("files", "skipped", "warnings", "ignore_sources"):
        out[field] = [normalize_paths(dict(item)) for item in out.get(field) or []]
    return out


def normalize_paths(item: dict[str, Any]) -> dict[str, Any]:
    for field in ("relative", "location"):
        if isinstance(item.get(field), str):
            item[field] = slash(item[field])
    return item


def slash(value: str) -> str:
    return value.replace("\\", "/")


def canonical(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def compare_drivers(
    go_binary: Path,
    rust_binary: Path,
    corpus: Path,
    symlinks: bool,
) -> dict[str, Any]:
    results: dict[str, Any] = {}
    normalized: dict[str, dict[str, Any]] = {"go": {}, "rust": {}}
    for operation in OPERATIONS:
        left = normalize(driver_payload(go_binary, operation, corpus), operation)
        right = normalize(driver_payload(rust_binary, operation, corpus), operation)
        if left != right:
            raise AssertionError(
                f"{operation} parity failed\nGo: {canonical(left)}\nRust: {canonical(right)}"
            )
        normalized["go"][operation] = left
        normalized["rust"][operation] = right
        results[operation] = {"status": "PASS", "items": item_count(left, operation)}
    if symlinks:
        options = {"follow_links": True}
        left = normalize(driver_payload(go_binary, "raw_walk_serial", corpus, options), "raw_walk_serial")
        right = normalize(driver_payload(rust_binary, "raw_walk_serial", corpus, options), "raw_walk_serial")
        if left != right:
            raise AssertionError(
                f"raw_walk_follow parity failed\nGo: {canonical(left)}\nRust: {canonical(right)}"
            )
        results["raw_walk_follow"] = {"status": "PASS", "items": len(left)}
    full_paths = [item["relative"] for item in normalized["go"]["scan"]["files"]]
    compact_paths = [item["relative"] for item in normalized["go"]["scan_compact"]["files"]]
    if full_paths != compact_paths:
        raise AssertionError("scan and scan_compact selected different files")
    return results


def item_count(value: Any, operation: str) -> int:
    if operation in ("scan", "scan_compact"):
        return len(value["files"])
    return len(value)


def go_competitor_results() -> dict[str, Any]:
    output = run(["go", "test", "-json", "-count=1", "./..."], cwd=GO_COMPAT)
    passed: list[str] = []
    skipped: list[str] = []
    differences: list[str] = []
    for line in output.splitlines():
        event = json.loads(line)
        test = event.get("Test")
        action = event.get("Action")
        if test and action == "pass":
            passed.append(test)
        elif test and action == "skip":
            skipped.append(test)
        message = event.get("Output", "").strip()
        if "documented difference:" in message:
            differences.append(message.split("documented difference:", 1)[1].strip())
    return {
        "status": "PASS",
        "passed": sorted(set(passed)),
        "skipped": sorted(set(skipped)),
        "documented_differences": sorted(set(differences)),
    }


def command_version(command: list[str]) -> str:
    return run(command).strip()


def filesystem_name(path: Path) -> str:
    if os.name != "nt":
        try:
            return run(["stat", "-f", "-c", "%T", str(path)]).strip()
        except RuntimeError:
            return "unknown"
    root = Path(path.anchor)
    name = ctypes.create_unicode_buffer(64)
    ok = ctypes.windll.kernel32.GetVolumeInformationW(
        str(root), None, 0, None, None, None, name, len(name)
    )
    return name.value if ok else "unknown"


def campaign() -> dict[str, Any]:
    with tempfile.TemporaryDirectory(prefix="treestamp-parity-") as directory:
        work = Path(directory)
        corpus = work / "corpus"
        corpus.mkdir()
        symlinks, symlink_error = write_fixture(corpus)
        go_binary, rust_binary = build_drivers(work)
        oracle = compare_drivers(go_binary, rust_binary, corpus, symlinks)
        competitors = go_competitor_results()
        return {
            "schema": 1,
            "campaign": "functional-parity",
            "status": "PASS",
            "recorded_at": datetime.now(timezone.utc).isoformat(),
            "platform": {
                "os": platform.platform(),
                "filesystem": filesystem_name(corpus),
                "architecture": platform.machine(),
                "container": Path("/.dockerenv").exists(),
            },
            "toolchain": {
                "go": command_version(["go", "version"]),
                "rustc": command_version(["rustc", "--version"]),
                "cargo": command_version(["cargo", "--version"]),
            },
            "pins": {
                "weavatrix_scan": "0.5.2@29c003a6ad541c9a10faf30505235375fa78b9d8",
                "fastwalk": "v1.0.14",
                "gocodewalker": "v1.5.1",
                "godirwalk": "v1.17.0",
            },
            "fixture": fixture_fingerprint(corpus),
            "capabilities": {
                "symlinks": symlinks,
                "symlink_error": symlink_error,
            },
            "rust_oracle": oracle,
            "go_competitors": competitors,
            "benchmark_timings": "NOT_RUN",
        }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    try:
        result = campaign()
    except (AssertionError, RuntimeError, subprocess.TimeoutExpired) as error:
        print(f"PARITY_FAIL: {error}")
        return 1
    text = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(text, encoding="utf-8", newline="\n")
    print(text, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
