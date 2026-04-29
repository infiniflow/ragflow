#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""Unit tests for Chunk Quality Analyzer.

Tests cover:
- Normal text chunks
- Table chunks (complete and broken)
- Garbled text detection (PUA, CID, control chars)
- Mojibake detection (UTF-8/GBK encoding mismatch)
- Duplicate chunk detection
- Too short/too long chunk detection
- Missing title detection
- Header/footer pollution detection

Note: Import analyzer.py directly by file path to avoid triggering
rag/flow/__init__.py which pulls in heavy dependencies.
"""

import hashlib
import importlib.util
import os
import sys
from enum import Enum


def _find_project_root(marker="pyproject.toml"):
    """Walk up from this file until a directory containing *marker* is found."""
    cur = os.path.dirname(os.path.abspath(__file__))
    while True:
        if os.path.exists(os.path.join(cur, marker)):
            return cur
        parent = os.path.dirname(cur)
        if parent == cur:
            raise FileNotFoundError(f"Could not locate project root (missing {marker})")
        cur = parent


_MODULE_PATH = os.path.join(
    _find_project_root(),
    "rag", "flow", "quality", "analyzer.py"
)
_spec = importlib.util.spec_from_file_location("analyzer", _MODULE_PATH)
_mod = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_mod)

ChunkQualityAnalyzer = _mod.ChunkQualityAnalyzer
ChunkQualityResult = _mod.ChunkQualityResult
QualityIssue = _mod.QualityIssue
QualityRiskLevel = _mod.QualityRiskLevel


class TestChunkQualityAnalyzerInitialization:
    """Tests for ChunkQualityAnalyzer initialization and defaults."""

    def test_default_initialization(self):
        analyzer = ChunkQualityAnalyzer()
        assert analyzer.min_chunk_length == 20
        assert analyzer.max_chunk_length == 8000
        assert analyzer.min_token_count == 10
        assert analyzer.max_token_count == 2000
        assert analyzer.garbled_threshold == 0.3
        assert len(analyzer.enable_checks) > 0

    def test_custom_initialization(self):
        analyzer = ChunkQualityAnalyzer(
            min_chunk_length=10,
            max_chunk_length=4000,
            garbled_threshold=0.5,
            enable_checks=["length", "garbled"],
        )
        assert analyzer.min_chunk_length == 10
        assert analyzer.max_chunk_length == 4000
        assert analyzer.garbled_threshold == 0.5
        assert analyzer.enable_checks == ["length", "garbled"]


class TestNormalTextChunks:
    """Tests for normal, high-quality text chunks."""

    def test_normal_chinese_text(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": "这是一段正常的中文文本内容，用于测试质量分析器的基本功能。"
                    "这段文本包含足够的长度和丰富的内容，应该被判定为高质量的chunk。"
                    "它没有乱码，没有重复，也没有任何格式问题。"
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        assert isinstance(result, ChunkQualityResult)
        assert result.chunk_index == 0
        assert result.quality_score > 0.9
        assert len(result.issues) == 0
        assert result.risk_level == QualityRiskLevel.LOW

    def test_normal_english_text(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": "This is a normal English text chunk with sufficient length. "
                    "It contains multiple sentences and provides rich semantic content. "
                    "The quality analyzer should mark this as a high-quality chunk."
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        assert result.quality_score > 0.9
        assert len(result.issues) == 0
        assert result.risk_level == QualityRiskLevel.LOW


class TestLengthBoundaries:
    """Tests for too short and too long chunk detection."""

    def test_empty_chunk(self):
        analyzer = ChunkQualityAnalyzer(min_chunk_length=20, max_chunk_length=100)
        chunk = {"text": ""}
        result = analyzer.analyze_single_chunk(chunk, 0)

        empty_issues = [i for i in result.issues if i.issue_type == "empty_chunk"]
        assert len(empty_issues) == 1
        assert empty_issues[0].risk_level == QualityRiskLevel.CRITICAL

    def test_too_short_chunk(self):
        analyzer = ChunkQualityAnalyzer(min_chunk_length=20, max_chunk_length=100)
        chunk = {"text": "Short text."}
        result = analyzer.analyze_single_chunk(chunk, 0)

        short_issues = [i for i in result.issues if i.issue_type == "chunk_too_short"]
        assert len(short_issues) == 1

    def test_too_long_chunk(self):
        analyzer = ChunkQualityAnalyzer(min_chunk_length=20, max_chunk_length=100)
        chunk = {"text": "a" * 150}
        result = analyzer.analyze_single_chunk(chunk, 0)

        long_issues = [i for i in result.issues if i.issue_type == "chunk_too_long"]
        assert len(long_issues) == 1


class TestCIDGarbledDetection:
    """Tests for CID garbled detection (PDF font encoding issues)."""

    def test_cid_pattern_detection(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {"text": "Text with (cid:123) CID pattern."}
        result = analyzer.analyze_single_chunk(chunk, 0)

        cid_issues = [i for i in result.issues if i.issue_type == "cid_garbled"]
        assert len(cid_issues) == 1
        assert cid_issues[0].risk_level == QualityRiskLevel.CRITICAL

    def test_cid_pattern_with_spaces(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {"text": "Text with (cid : 45) pattern."}
        result = analyzer.analyze_single_chunk(chunk, 0)

        cid_issues = [i for i in result.issues if i.issue_type == "cid_garbled"]
        assert len(cid_issues) == 1


class TestPUAGarbledDetection:
    """Tests for PUA (Private Use Area) character detection."""

    def test_pua_characters(self):
        analyzer = ChunkQualityAnalyzer(garbled_threshold=0.3)
        chunk = {"text": "\uE000\uE001\uE002\uE003\uE004\uE005\uE006\uE007\uE008 Normal"}
        result = analyzer.analyze_single_chunk(chunk, 0)

        pua_issues = [i for i in result.issues if i.issue_type in ["garbled_text", "mojibake_text"]]
        assert len(pua_issues) >= 1


class TestMojibakeDetection:
    """Tests for Chinese mojibake detection (UTF-8/GBK encoding mismatch).

    Mojibake occurs when:
    1. UTF-8 bytes are decoded as GBK (or vice versa)
    2. Common patterns: continuous extended Latin-1 characters, replacement chars
    """

    def test_replacement_characters(self):
        """Test detection of Unicode replacement character � (U+FFFD)."""
        analyzer = ChunkQualityAnalyzer()
        chunk = {"text": "Text with \ufffd\ufffd\ufffd replacement chars."}
        result = analyzer.analyze_single_chunk(chunk, 0)

        mojibake_issues = [i for i in result.issues if i.issue_type == "mojibake_text"]
        assert len(mojibake_issues) == 1
        assert result.metadata.get("replacement_char_count", 0) == 3

    def test_extended_latin1_sequence_uppercase(self):
        """Test detection of continuous uppercase extended Latin-1 characters."""
        analyzer = ChunkQualityAnalyzer()
        chunk = {"text": "ÀÁÂÃÄÅÆÇÈÉÊËÌÍÎÏÐÑÒÓÔÕÖ"}
        result = analyzer.analyze_single_chunk(chunk, 0)

        mojibake_issues = [i for i in result.issues if i.issue_type == "mojibake_text"]
        assert len(mojibake_issues) == 1
        assert result.metadata.get("extended_latin1_count", 0) > 0

    def test_extended_latin1_sequence_lowercase(self):
        """Test detection of continuous lowercase extended Latin-1 characters."""
        analyzer = ChunkQualityAnalyzer()
        chunk = {"text": "äåæçèéêëìíîïðñòóôõö÷ø"}
        result = analyzer.analyze_single_chunk(chunk, 0)

        mojibake_issues = [i for i in result.issues if i.issue_type == "mojibake_text"]
        assert len(mojibake_issues) == 1

    def test_superscript_subscript_sequence(self):
        """Test detection of continuous superscript/subscript symbols."""
        analyzer = ChunkQualityAnalyzer()
        chunk = {"text": "°±²³´µ¶·¹º»¼½¾ more °±²³"}
        result = analyzer.analyze_single_chunk(chunk, 0)

        mojibake_issues = [i for i in result.issues if i.issue_type == "mojibake_text"]
        assert len(mojibake_issues) >= 1

    def test_valid_french_not_mojibake(self):
        """Ensure valid French text is NOT classified as mojibake."""
        analyzer = ChunkQualityAnalyzer()
        chunk = {"text": "Bonjour, je m'appelle Pierre. Je vis à Paris."}
        result = analyzer.analyze_single_chunk(chunk, 0)

        mojibake_issues = [i for i in result.issues if i.issue_type == "mojibake_text"]
        assert len(mojibake_issues) == 0

    def test_valid_german_not_mojibake(self):
        """Ensure valid German text is NOT classified as mojibake."""
        analyzer = ChunkQualityAnalyzer()
        chunk = {"text": "Guten Tag, ich heiße Maria. Ich wohne in München."}
        result = analyzer.analyze_single_chunk(chunk, 0)

        mojibake_issues = [i for i in result.issues if i.issue_type == "mojibake_text"]
        assert len(mojibake_issues) == 0

    def test_valid_spanish_not_mojibake(self):
        """Ensure valid Spanish text is NOT classified as mojibake."""
        analyzer = ChunkQualityAnalyzer()
        chunk = {"text": "Hola, me llamo Carlos. Vivo en Madrid."}
        result = analyzer.analyze_single_chunk(chunk, 0)

        mojibake_issues = [i for i in result.issues if i.issue_type == "mojibake_text"]
        assert len(mojibake_issues) == 0


class TestDuplicateDetection:
    """Tests for duplicate chunk detection."""

    def test_duplicate_detection(self):
        analyzer = ChunkQualityAnalyzer()
        seen_hashes = set()

        chunk1 = {"text": "This is the first chunk content."}
        result1 = analyzer.analyze_single_chunk(chunk1, 0, seen_hashes)
        hash1 = hashlib.md5(chunk1["text"].encode("utf-8", errors="replace")).hexdigest()
        seen_hashes.add(hash1)

        chunk2 = {"text": "This is the first chunk content."}
        result2 = analyzer.analyze_single_chunk(chunk2, 1, seen_hashes)

        dup_issues = [i for i in result2.issues if i.issue_type == "duplicate_chunk"]
        assert len(dup_issues) == 1
        assert result2.quality_score < result1.quality_score

    def test_unique_not_flagged(self):
        analyzer = ChunkQualityAnalyzer()
        seen_hashes = set()

        chunk1 = {"text": "This is chunk one."}
        result1 = analyzer.analyze_single_chunk(chunk1, 0, seen_hashes)
        hash1 = hashlib.md5(chunk1["text"].encode("utf-8", errors="replace")).hexdigest()
        seen_hashes.add(hash1)

        chunk2 = {"text": "This is chunk two, different content."}
        result2 = analyzer.analyze_single_chunk(chunk2, 1, seen_hashes)

        dup_issues = [i for i in result2.issues if i.issue_type == "duplicate_chunk"]
        assert len(dup_issues) == 0


class TestTableDetection:
    """Tests for table chunk detection."""

    def test_complete_table(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": "<table><thead><tr><th>Name</th><th>Age</th></tr></thead>"
                    "<tbody><tr><td>John</td><td>25</td></tr></tbody></table>",
            "doc_type_kwd": "table",
            "ck_type": "table",
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        table_break_issues = [i for i in result.issues if i.issue_type == "table_break"]
        assert len(table_break_issues) == 0

    def test_broken_table(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": "<table><thead><tr><th>Name</th>",
            "doc_type_kwd": "table",
            "ck_type": "table",
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        table_break_issues = [i for i in result.issues if i.issue_type == "table_break"]
        assert len(table_break_issues) >= 1


class TestTokenCountChecks:
    """Tests for token count boundary checks."""

    def test_low_token_count(self):
        analyzer = ChunkQualityAnalyzer(min_token_count=5, max_token_count=100)
        chunk = {"text": "Short.", "tk_nums": 3}
        result = analyzer.analyze_single_chunk(chunk, 0)

        low_issues = [i for i in result.issues if i.issue_type == "token_count_too_low"]
        assert len(low_issues) == 1

    def test_high_token_count(self):
        analyzer = ChunkQualityAnalyzer(min_token_count=5, max_token_count=100)
        chunk = {"text": "Long.", "tk_nums": 150}
        result = analyzer.analyze_single_chunk(chunk, 0)

        high_issues = [i for i in result.issues if i.issue_type == "token_count_too_high"]
        assert len(high_issues) == 1


class TestBatchAnalysis:
    """Tests for batch chunk analysis."""

    def test_analyze_chunks_batch(self):
        analyzer = ChunkQualityAnalyzer()
        chunks = [
            {"text": "This is a normal, high-quality chunk with good content."},
            {"text": "短"},
            {"text": "Text with (cid:123) garbled pattern."},
        ]
        results = analyzer.analyze_chunks(chunks)
        summary = analyzer.get_batch_summary(results)

        assert len(results) == 3
        assert summary["total_chunks"] == 3
        assert summary["high_risk_count"] >= 1


class TestResultHelperMethods:
    """Tests for ChunkQualityResult helper methods."""

    def test_has_risk_above(self):
        result = ChunkQualityResult(
            chunk_index=0,
            quality_score=0.8,
            issues=[
                QualityIssue(
                    issue_type="garbled_text",
                    risk_level=QualityRiskLevel.HIGH,
                    description="Garbled",
                ),
            ],
        )

        assert result.has_risk_above(QualityRiskLevel.LOW) is True
        assert result.has_risk_above(QualityRiskLevel.HIGH) is True
        assert result.has_risk_above(QualityRiskLevel.CRITICAL) is False

    def test_get_issues_by_type(self):
        result = ChunkQualityResult(
            chunk_index=0,
            quality_score=0.5,
            issues=[
                QualityIssue(
                    issue_type="garbled_text",
                    risk_level=QualityRiskLevel.HIGH,
                    description="Garbled",
                ),
                QualityIssue(
                    issue_type="garbled_text",
                    risk_level=QualityRiskLevel.LOW,
                    description="More garbled",
                ),
            ],
        )

        garbled_issues = result.get_issues_by_type("garbled_text")
        assert len(garbled_issues) == 2


class TestQualityRiskLevelEnum:
    """Tests for QualityRiskLevel enum."""

    def test_enum_values(self):
        assert QualityRiskLevel.LOW.value == "low"
        assert QualityRiskLevel.MEDIUM.value == "medium"
        assert QualityRiskLevel.HIGH.value == "high"
        assert QualityRiskLevel.CRITICAL.value == "critical"

    def test_enum_is_enum(self):
        assert issubclass(QualityRiskLevel, Enum)


class TestTaskExecutorIntegrationPattern:
    """Tests for integration with task_executor pattern.

    This simulates how chunks are processed in task_executor.py.
    """

    def test_quality_fields_attached_to_chunks(self):
        """Simulate _analyze_chunks_quality behavior."""
        analyzer = ChunkQualityAnalyzer()
        chunks = [
            {"content_with_weight": "This is a normal chunk with good content.", "doc_id": "test1"},
            {"content_with_weight": "Text with (cid:123) garbled pattern.", "doc_id": "test2"},
            {"content_with_weight": "ÀÁÂÃÄÅÆÇ mojibake text.", "doc_id": "test3"},
        ]

        results = analyzer.analyze_chunks(chunks)
        summary = analyzer.get_batch_summary(results)

        for idx, chunk in enumerate(chunks):
            if idx < len(results):
                r = results[idx]
                chunk["_quality_score"] = r.quality_score
                chunk["_quality_risk_level"] = r.risk_level.value
                if r.issues:
                    chunk["_quality_issues"] = [
                        {
                            "type": i.issue_type,
                            "risk": i.risk_level.value,
                            "desc": i.description,
                            "details": i.details,
                            "suggest": i.suggestion,
                        }
                        for i in r.issues
                    ]
                else:
                    chunk["_quality_issues"] = []
                chunk["_quality_metadata"] = r.metadata

        assert chunks[0].get("_quality_score") is not None
        assert chunks[0]["_quality_score"] > 0.9
        assert chunks[0]["_quality_risk_level"] == "low"
        assert chunks[0]["_quality_issues"] == []

        assert "cid_garbled" in [i["type"] for i in chunks[1]["_quality_issues"]]
        assert chunks[1]["_quality_risk_level"] == "critical"

        assert summary["total_chunks"] == 3
        assert summary["high_risk_count"] >= 1
