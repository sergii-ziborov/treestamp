#!/usr/bin/env python3
"""Create the personal public GitHub repository sergii-ziborov/treestamp."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REQUIRED_LOGIN = "sergii-ziborov"
FORBIDDEN_ORGS = {"Weavatrix", "weavatrix", "EdgeHawk", "edgehawk", "EDGEHAWK"}
REPO_NAME = "treestamp"
DESCRIPTION = "Native Go port of Weavatrix Scan. Personal public repository of Sergii Ziborov."


def run(args: list[str], check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(args, cwd=ROOT, check=check, text=True, capture_output=True)


def gh_login() -> str:
    completed = run(["gh", "api", "user"])
    payload = json.loads(completed.stdout)
    return payload["login"]


def existing_remote() -> str | None:
    completed = run(["git", "remote", "get-url", "origin"], check=False)
    if completed.returncode != 0:
        return None
    return completed.stdout.strip()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--visibility", choices=("public", "private"), default="public")
    parser.add_argument("--org", help="Rejected unless omitted. This repo is personal.")
    parser.add_argument("--execute", action="store_true")
    args = parser.parse_args()

    if args.org:
        print(f"refusing organization {args.org!r}: this repository is personal", file=sys.stderr)
        return 2
    if args.visibility != "public":
        print("this project is specified as a public personal repository", file=sys.stderr)
        return 2

    try:
        login = gh_login()
    except (subprocess.CalledProcessError, FileNotFoundError) as error:
        print(f"gh is required and must be logged in: {error}", file=sys.stderr)
        return 1
    if login != REQUIRED_LOGIN:
        print(
            f"logged in as {login!r}, expected {REQUIRED_LOGIN!r}. "
            "Will not create the repository on another account.",
            file=sys.stderr,
        )
        return 2
    if login in FORBIDDEN_ORGS:
        print("refusing Weavatrix/EdgeHawk hosting", file=sys.stderr)
        return 2

    remote = existing_remote()
    plan = {
        "owner": login,
        "name": REPO_NAME,
        "visibility": "public",
        "module": "github.com/sergii-ziborov/treestamp",
        "execute": args.execute,
        "existing_origin": remote,
        "not": ["Weavatrix", "EdgeHawk"],
    }
    print(json.dumps(plan, indent=2))
    if not args.execute:
        print("dry run only; pass --execute to create and push")
        return 0
    if remote:
        print("origin already exists; refusing to overwrite or force-push", file=sys.stderr)
        return 2

    created = run(
        [
            "gh",
            "repo",
            "create",
            f"{REQUIRED_LOGIN}/{REPO_NAME}",
            "--public",
            "--description",
            DESCRIPTION,
            "--source",
            str(ROOT),
            "--remote",
            "origin",
            "--push",
        ],
        check=False,
    )
    sys.stdout.write(created.stdout)
    sys.stderr.write(created.stderr)
    return created.returncode


if __name__ == "__main__":
    raise SystemExit(main())
