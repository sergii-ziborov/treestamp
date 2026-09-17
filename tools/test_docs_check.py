import json
import tempfile
import unittest
from pathlib import Path

import docs_check


class DocsCheckTests(unittest.TestCase):
    def test_catalog_regions_exist(self):
        errors = docs_check.check_structure()
        self.assertEqual(errors, [])

    def test_region_extracts_example(self):
        source = docs_check.example_source()
        blob = docs_check.region(source, "example-explain")
        self.assertIn("func ExampleScanner_Explain(", blob)

    def test_missing_region(self):
        with self.assertRaises(SystemExit):
            docs_check.region("package x\n", "nope")

    def test_parse_events_requires_pass(self):
        events = docs_check.parse_example_events(
            [
                '{"Action":"pass","Test":"ExampleScanPaths"}',
                '{"Action":"skip","Test":"ExampleWalkFS"}',
            ]
        )
        self.assertEqual(events["ExampleScanPaths"], "pass")
        self.assertEqual(events["ExampleWalkFS"], "skip")

    def test_check_events_flags_skip(self):
        catalog = docs_check.load_catalog()
        lines = [json.dumps({"Action": "pass", "Test": item["name"]}) for item in catalog["examples"]]
        lines[0] = json.dumps({"Action": "skip", "Test": catalog["examples"][0]["name"]})
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "events.jsonl"
            path.write_text("\n".join(lines), encoding="utf-8")
            errors = docs_check.check_events(path)
        self.assertTrue(any("skip" in item for item in errors))

    def test_check_events_flags_missing(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "events.jsonl"
            path.write_text("{}\n", encoding="utf-8")
            errors = docs_check.check_events(path)
        self.assertTrue(any("did not run" in item for item in errors))

    def test_index_lists_troubleshooting(self):
        text = (docs_check.ROOT / "docs" / "index.md").read_text(encoding="utf-8")
        self.assertIn("troubleshooting.md", text)

    def test_quickstart_exists(self):
        self.assertTrue((docs_check.ROOT / "examples" / "docquickstart" / "main.go").is_file())

    def test_agent_guide_forbids_internal(self):
        text = (docs_check.ROOT / "docs" / "agent-guide.md").read_text(encoding="utf-8")
        self.assertIn("internal/", text)

    def test_readme_mentions_official_measured(self):
        text = (docs_check.ROOT / "README.md").read_text(encoding="utf-8")
        self.assertIn("MEASURED", text)
        self.assertIn("official", text.lower())


if __name__ == "__main__":
    unittest.main()
