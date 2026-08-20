import importlib.util
import os
import sys
import unittest

# Load module directly
file_path = os.path.join(
    os.path.dirname(__file__),
    "../../api/utils/production_debt.py",
)
spec = importlib.util.spec_from_file_location("ragflow_production_debt", file_path)
production_debt_mod = importlib.util.module_from_spec(spec)
sys.modules["ragflow_production_debt"] = production_debt_mod
spec.loader.exec_module(production_debt_mod)

ProductionDebtRAGEvaluator = production_debt_mod.ProductionDebtRAGEvaluator
TechnicalDueDiligenceLedger = production_debt_mod.TechnicalDueDiligenceLedger
GENESIS_HASH = production_debt_mod.GENESIS_HASH


class TestProductionDebtRAGEvaluator(unittest.TestCase):
    def setUp(self) -> None:
        self.evaluator = ProductionDebtRAGEvaluator(
            never_equate_intent_to_approval=True,
            max_acceptable_rdi=12.0,
        )

    def test_clean_pipeline_passes_readiness(self) -> None:
        report = self.evaluator.evaluate_pipeline(
            dataset_id="dataset_sec_filings",
            pipeline_id="pipe_10k_parsing",
            raw_document_bytes=1000000,
            chunk_storage_bytes=1050000,
            retrieval_latency_seconds=0.85,
            multi_hop_loop_count=0,
            un_gated_mutations=0,
        )
        self.assertTrue(report.is_production_ready)
        self.assertLessEqual(report.rdi_score, 12.0)
        self.assertEqual(len(report.critical_smells), 0)
        self.assertTrue(bool(report.receipt_hash))

    def test_degraded_pipeline_fails_debt(self) -> None:
        report = self.evaluator.evaluate_pipeline(
            dataset_id="dataset_legacy_scans",
            pipeline_id="pipe_fragmented_ocr",
            raw_document_bytes=1000000,
            chunk_storage_bytes=4200000,  # High chunk fragmentation (4.2x)
            retrieval_latency_seconds=8.5,  # High latency
            multi_hop_loop_count=5,  # 5 loops
            un_gated_mutations=2,  # 2 un-gated mutations
        )
        self.assertFalse(report.is_production_ready)
        self.assertGreater(report.rdi_score, 50.0)
        self.assertIn("HIGH_CHUNK_FRAGMENTATION_4.20X", report.critical_smells)
        self.assertIn("HIGH_RETRIEVAL_LATENCY_8.50S", report.critical_smells)
        self.assertIn("DETECTED_5_MULTI_HOP_LOOPS", report.critical_smells)
        self.assertIn("DETECTED_2_UNGATED_MUTATIONS", report.critical_smells)

    def test_cryptographic_ledger_integrity(self) -> None:
        self.evaluator.evaluate_pipeline("ds-1", "p-1")
        self.evaluator.evaluate_pipeline("ds-2", "p-2")
        self.evaluator.evaluate_pipeline("ds-3", "p-3")

        entries = self.evaluator.ledger.get_ledger_entries()
        self.assertEqual(len(entries), 3)
        self.assertEqual(entries[0]["prev_hash"], GENESIS_HASH)
        self.assertEqual(entries[1]["prev_hash"], entries[0]["curr_hash"])
        self.assertEqual(entries[2]["prev_hash"], entries[1]["curr_hash"])
        self.assertTrue(self.evaluator.ledger.verify_ledger_integrity())


if __name__ == "__main__":
    unittest.main()
