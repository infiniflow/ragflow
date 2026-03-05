"""
Data verification for RAGFlow migration.
"""

import json
import logging
from dataclasses import dataclass, field
from typing import Any

from .es_client import ESClient
from .ob_client import OBClient
from .schema import ARRAY_COLUMNS, JSON_COLUMNS

logger = logging.getLogger(__name__)


@dataclass
class VerificationResult:
    """Migration verification result."""

    es_index: str
    ob_table: str

    # Counts
    es_count: int = 0
    ob_count: int = 0
    count_match: bool = False
    count_diff: int = 0

    # Sample verification
    sample_size: int = 0
    samples_verified: int = 0
    samples_matched: int = 0
    sample_match_rate: float = 0.0

    # Mismatches
    missing_in_ob: list[str] = field(default_factory=list)
    data_mismatches: list[dict[str, Any]] = field(default_factory=list)

    # Overall
    passed: bool = False
    message: str = ""


class MigrationVerifier:
    """Verify RAGFlow migration data consistency."""

    # Fields to compare for verification
    VERIFY_FIELDS = [
        "id", "kb_id", "doc_id", "docnm_kwd", "content_with_weight",
        "available_int", "create_time",
    ]

    def __init__(
        self,
        es_client: ESClient,
        ob_client: OBClient,
    ):
        """
        Initialize verifier.

        Args:
            es_client: Elasticsearch client
            ob_client: OceanBase client
        """
        self.es_client = es_client
        self.ob_client = ob_client

    def verify(
        self,
        es_index: str,
        ob_table: str,
        sample_size: int = 100,
        primary_key: str = "id",
        verify_fields: list[str] | None = None,
    ) -> VerificationResult:
        """
        Verify migration by comparing ES and OceanBase data.

        Args:
            es_index: Elasticsearch index name
            ob_table: OceanBase table name
            sample_size: Number of documents to sample for verification
            primary_key: Primary key column name
            verify_fields: Fields to verify (None = use defaults)

        Returns:
            VerificationResult with details
        """
        result = VerificationResult(
            es_index=es_index, 
            ob_table=ob_table,
        )

        if verify_fields is None:
            verify_fields = self.VERIFY_FIELDS

        # Step 1: Verify document counts
        logger.info("Verifying document counts...")
        
        result.es_count = self.es_client.count_documents(es_index)
        result.ob_count = self.ob_client.count_rows(ob_table)
        
        result.count_diff = abs(result.es_count - result.ob_count)
        result.count_match = result.count_diff == 0

        logger.info(
            f"Document counts - ES: {result.es_count}, OB: {result.ob_count}, "
            f"Diff: {result.count_diff}"
        )

        # Step 2: Sample verification
        result.sample_size = min(sample_size, result.es_count)
        
        if result.sample_size > 0:
            logger.info(f"Verifying {result.sample_size} sample documents...")
            self._verify_samples(
                es_index, ob_table, result, primary_key, verify_fields
            )

        # Step 3: Determine overall result
        self._determine_result(result)

        logger.info(result.message)
        return result

    def _verify_samples(
        self,
        es_index: str,
        ob_table: str,
        result: VerificationResult,
        primary_key: str,
        verify_fields: list[str],
    ):
        """Verify sample documents."""
        # Get sample documents from ES
        es_samples = self.es_client.get_sample_documents(
            es_index, result.sample_size
        )

        for es_doc in es_samples:
            result.samples_verified += 1
            doc_id = es_doc.get("_id") or es_doc.get("id")

            if not doc_id:
                logger.warning("Document without ID found")
                continue

            # Get corresponding document from OceanBase
            ob_doc = self.ob_client.get_row_by_id(ob_table, doc_id)

            if ob_doc is None:
                result.missing_in_ob.append(doc_id)
                continue

            # Compare documents
            match, differences = self._compare_documents(
                es_doc, ob_doc, verify_fields
            )
            
            if match:
                result.samples_matched += 1
            else:
                result.data_mismatches.append({
                    "id": doc_id,
                    "differences": differences,
                })

        # Calculate match rate
        if result.samples_verified > 0:
            result.sample_match_rate = result.samples_matched / result.samples_verified

    def _compare_documents(
        self,
        es_doc: dict[str, Any],
        ob_doc: dict[str, Any],
        verify_fields: list[str],
    ) -> tuple[bool, list[dict[str, Any]]]:
        """
        Compare ES document with OceanBase row.
        
        Returns:
            Tuple of (match: bool, differences: list)
        """
        differences = []

        for field_name in verify_fields:
            es_value = es_doc.get(field_name)
            ob_value = ob_doc.get(field_name)

            # Skip if both are None/null
            if es_value is None and ob_value is None:
                continue

            # Handle special comparisons
            if not self._values_equal(field_name, es_value, ob_value):
                differences.append({
                    "field": field_name,
                    "es_value": es_value,
                    "ob_value": ob_value,
                })

        return len(differences) == 0, differences

    def _values_equal(
        self, 
        field_name: str, 
        es_value: Any, 
        ob_value: Any
    ) -> bool:
        """Compare two values with type-aware logic."""
        if es_value is None and ob_value is None:
            return True

        if es_value is None or ob_value is None:
            # One is None, the other isn't
            # For optional fields, this might be acceptable
            return False

        # Handle array fields (stored as JSON strings in OB)
        if field_name in ARRAY_COLUMNS:
            if isinstance(ob_value, str):
                try:
                    ob_value = json.loads(ob_value)
                except json.JSONDecodeError:
                    pass
            if isinstance(es_value, list) and isinstance(ob_value, list):
                return set(str(x) for x in es_value) == set(str(x) for x in ob_value)

        # Handle JSON fields
        if field_name in JSON_COLUMNS:
            if isinstance(ob_value, str):
                try:
                    ob_value = json.loads(ob_value)
                except json.JSONDecodeError:
                    pass
            if isinstance(es_value, str):
                try:
                    es_value = json.loads(es_value)
                except json.JSONDecodeError:
                    pass
            return es_value == ob_value

        # Handle content_with_weight which might be dict or string
        if field_name == "content_with_weight":
            if isinstance(ob_value, str) and isinstance(es_value, dict):
                try:
                    ob_value = json.loads(ob_value)
                except json.JSONDecodeError:
                    pass

        # Handle kb_id which might be list in ES
        if field_name == "kb_id":
            if isinstance(es_value, list) and len(es_value) > 0:
                es_value = es_value[0]

        # Standard comparison
        return str(es_value) == str(ob_value)

    def _determine_result(self, result: VerificationResult):
        """Determine overall verification result."""
        # Allow small count differences (e.g., documents added during migration)
        count_tolerance = 0.01  # 1% tolerance
        count_ok = (
            result.count_match or 
            (result.es_count > 0 and result.count_diff / result.es_count <= count_tolerance)
        )

        if count_ok and result.sample_match_rate >= 0.99:
            result.passed = True
            result.message = (
                f"Verification PASSED. "
                f"ES: {result.es_count:,}, OB: {result.ob_count:,}. "
                f"Sample match rate: {result.sample_match_rate:.2%}"
            )
        elif count_ok and result.sample_match_rate >= 0.95:
            result.passed = True
            result.message = (
                f"Verification PASSED with warnings. "
                f"ES: {result.es_count:,}, OB: {result.ob_count:,}. "
                f"Sample match rate: {result.sample_match_rate:.2%}"
            )
        else:
            result.passed = False
            issues = []
            if not count_ok:
                issues.append(
                    f"Count mismatch (ES: {result.es_count}, OB: {result.ob_count}, diff: {result.count_diff})"
                )
            if result.sample_match_rate < 0.95:
                issues.append(f"Low sample match rate: {result.sample_match_rate:.2%}")
            if result.missing_in_ob:
                issues.append(f"{len(result.missing_in_ob)} documents missing in OB")
            result.message = f"Verification FAILED: {'; '.join(issues)}"

    def generate_report(self, result: VerificationResult) -> str:
        """Generate a verification report."""
        lines = [
            "",
            "=" * 60,
            "Migration Verification Report",
            "=" * 60,
            f"ES Index:  {result.es_index}",
            f"OB Table:  {result.ob_table}",
        ]
        
        lines.extend([
            "",
            "Document Counts:",
            f"  Elasticsearch: {result.es_count:,}",
            f"  OceanBase:     {result.ob_count:,}",
            f"  Difference:    {result.count_diff:,}",
            f"  Match:         {'Yes' if result.count_match else 'No'}",
            "",
            "Sample Verification:",
            f"  Sample Size:   {result.sample_size}",
            f"  Verified:      {result.samples_verified}",
            f"  Matched:       {result.samples_matched}",
            f"  Match Rate:    {result.sample_match_rate:.2%}",
            "",
        ])

        if result.missing_in_ob:
            lines.append(f"Missing in OceanBase ({len(result.missing_in_ob)}):")
            for doc_id in result.missing_in_ob[:5]:
                lines.append(f"  - {doc_id}")
            if len(result.missing_in_ob) > 5:
                lines.append(f"  ... and {len(result.missing_in_ob) - 5} more")
            lines.append("")

        if result.data_mismatches:
            lines.append(f"Data Mismatches ({len(result.data_mismatches)}):")
            for mismatch in result.data_mismatches[:3]:
                lines.append(f"  - ID: {mismatch['id']}")
                for diff in mismatch.get("differences", [])[:2]:
                    lines.append(f"    {diff['field']}: ES={diff['es_value']}, OB={diff['ob_value']}")
            if len(result.data_mismatches) > 3:
                lines.append(f"  ... and {len(result.data_mismatches) - 3} more")
            lines.append("")

        lines.extend([
            "=" * 60,
            f"Result: {'PASSED' if result.passed else 'FAILED'}",
            result.message,
            "=" * 60,
            "",
        ])

        return "\n".join(lines)
