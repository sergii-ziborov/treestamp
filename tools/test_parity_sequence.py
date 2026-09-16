#!/usr/bin/env python3

from __future__ import annotations

import unittest

from parity_sequence import compare_step, file_hash, require_reuse


def identity(payload: dict, _operation: str):
    return payload["data"]


class SequenceHelperTests(unittest.TestCase):
    def test_file_hash_and_reuse(self) -> None:
        scan = {"files": [{"relative": "a.txt", "content_hash": "sha256:1"}]}
        self.assertEqual(file_hash(scan, "a.txt"), "sha256:1")
        payload = {"data": {"cache": {"reused_hashes": 3}}}
        require_reuse(payload, payload)

    def test_compare_step_reason(self) -> None:
        payload = {"data": {"files": [{"relative": "a.txt"}], "watch_reason": "Incremental"}}
        results: dict = {}
        compare_step("edit", payload, payload, "scan", identity, str, results, "Incremental")
        self.assertEqual(results["edit"]["status"], "PASS")

    def test_reuse_must_be_nonzero(self) -> None:
        empty = {"data": {"cache": {"reused_hashes": 0}}}
        with self.assertRaises(AssertionError):
            require_reuse(empty, empty)


if __name__ == "__main__":
    unittest.main()
