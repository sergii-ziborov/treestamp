#!/usr/bin/env python3
"""Build the sample as an external module. Source mode uses a local replace."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def run(cmd: list[str], cwd: Path, extra_env: dict[str, str] | None = None) -> None:
    env = os.environ.copy()
    env["GOWORK"] = "off"
    env["CGO_ENABLED"] = "0"
    env["GOTOOLCHAIN"] = env.get("GOTOOLCHAIN", "local")
    if extra_env:
        env.update(extra_env)
    subprocess.check_call(cmd, cwd=cwd, env=env)


def tagged(version: str) -> str:
    version = version.strip()
    if not version.startswith("v"):
        return "v" + version
    return version


def write_consumer(dest: Path, require: str, replace: str | None) -> None:
    lines = [
        "module consumercheck",
        "",
        "go 1.21.0",
        "",
        f"require github.com/sergii-ziborov/treestamp {require}",
        "",
    ]
    if replace:
        lines.extend([f"replace github.com/sergii-ziborov/treestamp => {replace}", ""])
    (dest / "go.mod").write_text("\n".join(lines), encoding="utf-8")
    (dest / "main.go").write_text(
        (ROOT / "examples" / "docquickstart" / "main.go").read_text(encoding="utf-8"),
        encoding="utf-8",
    )


def source_consumer() -> None:
    with tempfile.TemporaryDirectory(prefix="treestamp-consumer-") as tmp:
        dest = Path(tmp)
        write_consumer(dest, "v0.0.0", ROOT.as_posix())
        run(["go", "mod", "tidy"], dest)
        run(["go", "run", ".", "-root", str(ROOT / "examples" / "docquickstart")], dest)


def release_consumer(version: str) -> None:
    version = tagged(version)
    with tempfile.TemporaryDirectory(prefix="treestamp-release-consumer-") as tmp:
        dest = Path(tmp)
        write_consumer(dest, version, None)
        run(["go", "mod", "tidy"], dest)
        run(["go", "run", ".", "-root", str(ROOT / "examples" / "docquickstart")], dest)


def installed_cli(version: str) -> None:
    version = tagged(version)
    with tempfile.TemporaryDirectory(prefix="treestamp-cli-install-") as tmp:
        dest = Path(tmp)
        gobin = dest / "bin"
        gobin.mkdir()
        run(
            ["go", "install", f"github.com/sergii-ziborov/treestamp/cmd/treestamp@{version}"],
            dest,
            {"GOBIN": str(gobin)},
        )
        name = "treestamp.exe" if os.name == "nt" else "treestamp"
        exe = gobin / name
        env = os.environ.copy()
        env["GOWORK"] = "off"
        env["CGO_ENABLED"] = "0"
        raw = subprocess.check_output([str(exe), "version", "--json"], env=env)
        doc = json.loads(raw)
        want = version[1:]
        if doc.get("cli") != want:
            raise SystemExit(f"installed CLI {doc.get('cli')!r} != {want!r}")
        for args in (
            ["scan", "--help"],
            ["verify", "--help"],
            ["paths", "--help"],
            ["explain", "--help"],
        ):
            subprocess.check_call([str(exe), *args], env=env)
        subprocess.check_call(["go", "version", "-m", str(exe)], env=env)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=("source", "release", "install"), default="source")
    parser.add_argument("--version")
    args = parser.parse_args()
    if args.mode == "source":
        source_consumer()
        print("SOURCE CONSUMER PASSED")
        return 0
    if not args.version:
        print(f"{args.mode} mode requires --version", file=sys.stderr)
        return 1
    if args.mode == "release":
        release_consumer(args.version)
        print("RELEASE CONSUMER PASSED")
        return 0
    installed_cli(args.version)
    print("INSTALLED CLI PASSED")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
