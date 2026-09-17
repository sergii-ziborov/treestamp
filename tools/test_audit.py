#!/usr/bin/env python3

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
AUDIT = ROOT / "tools" / "audit.py"


class AuditTests(unittest.TestCase):
    def test_bootstrap_audit_passes(self) -> None:
        completed = subprocess.run(
            [sys.executable, str(AUDIT)],
            cwd=ROOT,
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(completed.returncode, 0, completed.stderr)

    def test_require_full_passes_when_port_is_closed(self) -> None:
        completed = subprocess.run(
            [sys.executable, str(AUDIT), "--require-full"],
            cwd=ROOT,
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(completed.returncode, 0, completed.stderr)
        self.assertIn("FULL PORT CHECK PASSED", completed.stdout)

    def test_pin_and_contracts_are_complete(self) -> None:
        pin = json.loads((ROOT / "compat" / "pin.json").read_text(encoding="utf-8"))
        contracts = json.loads((ROOT / "compat" / "contracts.json").read_text(encoding="utf-8"))
        self.assertEqual(pin["upstream"]["commit"], "29c003a6ad541c9a10faf30505235375fa78b9d8")
        self.assertEqual(len(contracts["contracts"]), 35)

    def test_audit_script_is_not_a_scanner(self) -> None:
        source = AUDIT.read_text(encoding="utf-8")
        self.assertNotIn("def scan_repository", source)


if __name__ == "__main__":
    unittest.main()
