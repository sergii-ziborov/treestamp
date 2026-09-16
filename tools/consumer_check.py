#!/usr/bin/env python3
"""Build the sample as an external module. Source mode uses a local replace."""

from __future__ import annotations

import argparse
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


def source_consumer() -> None:
    with tempfile.TemporaryDirectory(prefix="treestamp-consumer-") as tmp:
        dest = Path(tmp)
        (dest / "go.mod").write_text(
            "\n".join(
                [
                    "module consumercheck",
                    "",
                    "go 1.23.0",
                    "",
                    "require github.com/sergii-ziborov/treestamp v0.0.0",
                    "",
                    f"replace github.com/sergii-ziborov/treestamp => {ROOT.as_posix()}",
                    "",
                ]
            ),
            encoding="utf-8",
        )
        (dest / "main.go").write_text(
            (ROOT / "examples" / "docquickstart" / "main.go").read_text(encoding="utf-8"),
            encoding="utf-8",
        )
        run(["go", "mod", "tidy"], dest)
        run(["go", "run", ".", "-root", str(ROOT / "examples" / "docquickstart")], dest)


def release_consumer(version: str) -> None:
    raise SystemExit(f"release consumer needs a published tag; got {version!r}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=("source", "release"), default="source")
    parser.add_argument("--version")
    args = parser.parse_args()
    if args.mode == "source":
        source_consumer()
        print("SOURCE CONSUMER PASSED")
        return 0
    if not args.version:
        print("release mode requires --version", file=sys.stderr)
        return 1
    release_consumer(args.version)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
