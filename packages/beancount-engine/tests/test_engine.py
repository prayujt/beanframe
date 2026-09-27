import json
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
from pathlib import Path

from beancount_engine.__main__ import snapshot
from beancount import loader

FIXTURE = Path(__file__).resolve().parents[3] / "testdata/ledger/main.beancount"


class EngineTests(unittest.TestCase):
    def test_includes_inferred_amounts_and_currencies(self):
        result = snapshot(FIXTURE)
        self.assertEqual(result["errors"], [])
        self.assertEqual(len(result["transactions"]), 4)
        self.assertEqual(result["transactions"][1]["postings"][1]["units"],
                         {"number": "12.34", "currency": "USD"})
        checking = next(b for b in result["balances"] if b["account"] == "Assets:Checking")
        self.assertEqual({p["units"]["currency"]: p["units"]["number"] for p in checking["positions"]},
                         {"USD": "117.66", "EUR": "-2.50"})

    def test_invalid_ledger_has_no_balances(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "main.beancount"
            path.write_text('2026-01-01 * "Broken"\n  Assets:Missing  1 USD\n')
            result = snapshot(path)
            self.assertTrue(result["errors"])
            self.assertEqual(result["balances"], [])

    def test_cost_lots_are_not_flattened(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "main.beancount"
            path.write_text('2026-01-01 open Assets:Broker\n2026-01-01 open Assets:Cash\n'
                            '2026-01-02 * "Buy"\n  Assets:Broker  2 TEST {10 USD}\n  Assets:Cash  -20 USD\n')
            result = snapshot(path)
            self.assertEqual(result["errors"], [])
            broker = next(b for b in result["balances"] if b["account"] == "Assets:Broker")
            self.assertEqual(broker["positions"][0]["units"], {"number": "2", "currency": "TEST"})
            self.assertIn("USD", broker["positions"][0]["cost"])

    def test_subprocess_protocol(self):
        result = subprocess.run([sys.executable, "-m", "beancount_engine"],
                                input=json.dumps({"operation": "snapshot", "path": str(FIXTURE)}),
                                text=True, capture_output=True, check=True)
        self.assertEqual(json.loads(result.stdout)["title"], "Example ledger")

    def test_loader_does_not_touch_adjacent_cache(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "main.beancount"
            path.write_text('2026-01-01 open Assets:Checking\n')
            cache = Path(directory) / ".main.beancount.picklecache"
            cache.write_bytes(b"existing unrelated cache")
            before = {p.name: p.read_bytes() for p in Path(directory).iterdir()}
            with patch.object(loader, "PICKLE_CACHE_THRESHOLD", -1):
                self.assertEqual(snapshot(path)["errors"], [])
            self.assertEqual(before, {p.name: p.read_bytes() for p in Path(directory).iterdir()})


if __name__ == "__main__":
    unittest.main()
