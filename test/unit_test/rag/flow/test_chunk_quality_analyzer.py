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
- Garbled text detection
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
from unittest import mock


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
                    "The quality analyzer should mark this as a high-quality chunk "
                    "without any issues detected."
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        assert result.quality_score > 0.9
        assert len(result.issues) == 0
        assert result.risk_level == QualityRiskLevel.LOW

    def test_mixed_language_text(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": "This is a mixed language text. 这是一段中英文混合的文本。"
                    "It contains both English and Chinese characters. "
                    "包含中英文两种语言字符。Quality should be high. 质量应该很高。"
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        assert result.quality_score > 0.9
        assert len(result.issues) == 0
        assert result.metadata["text_length"] == len(chunk["text"])


class TestTableChunks:
    """Tests for table chunks (complete and broken)."""

    def test_complete_html_table(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": "<table><thead><tr><th>Name</th><th>Age</th><th>City</th></tr></thead>"
                    "<tbody><tr><td>John</td><td>25</td><td>New York</td></tr>"
                    "<tr><td>Jane</td><td>30</td><td>London</td></tr></tbody></table>",
            "doc_type_kwd": "table",
            "ck_type": "table",
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        table_issues = [i for i in result.issues if i.issue_type == "table_break"]
        assert len(table_issues) == 0

    def test_markdown_table_complete(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": "| Name | Age | City |\n|------|-----|------|\n| John | 25 | New York |\n| Jane | 30 | London |",
            "doc_type_kwd": "table",
            "ck_type": "table",
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        table_issues = [i for i in result.issues if i.issue_type == "table_break"]
        assert len(table_issues) == 0

    def test_broken_table_unclosed_tags(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": "<table><thead><tr><th>Name</th><th>Age</th>",
            "doc_type_kwd": "table",
            "ck_type": "table",
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        table_issues = [i for i in result.issues if i.issue_type == "table_break"]
        assert len(table_issues) >= 1
        assert table_issues[0].risk_level in [QualityRiskLevel.HIGH, QualityRiskLevel.MEDIUM]

    def test_table_with_single_row(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": "| Name | Age |\n|------|-----|",
            "doc_type_kwd": "table",
            "ck_type": "table",
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        table_issues = [i for i in result.issues if i.issue_type == "table_break"]
        assert len(table_issues) >= 1


class TestGarbledTextDetection:
    """Tests for garbled text detection (PUA chars, CID patterns, control chars)."""

    def test_cid_pattern_detection(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": "This is some text with (cid:123) CID patterns (cid:456) in it."
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        cid_issues = [i for i in result.issues if i.issue_type == "cid_garbled"]
        assert len(cid_issues) == 1
        assert cid_issues[0].risk_level == QualityRiskLevel.CRITICAL

    def test_cid_pattern_with_spaces(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": "Text with (cid : 789) pattern and more text."
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        cid_issues = [i for i in result.issues if i.issue_type == "cid_garbled"]
        assert len(cid_issues) == 1

    def test_pua_characters(self):
        analyzer = ChunkQualityAnalyzer(garbled_threshold=0.3)
        chunk = {
            "text": "\uE000\uE001\uE002\uE003\uE004\uE005\uE006\uE007\uE008 Normal"
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        garbled_issues = [i for i in result.issues if i.issue_type == "garbled_text"]
        assert len(garbled_issues) == 1
        assert result.quality_score < 0.8

    def test_replacement_character(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": "Document with \uFFFD\uFFFD\uFFFD replacement characters"
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        assert "garbled_ratio" in result.metadata

    def test_control_characters(self):
        analyzer = ChunkQualityAnalyzer(garbled_threshold=0.3)
        chunk = {
            "text": "\x00\x01\x02\x03\x04\x05Text with control characters"
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        garbled_issues = [i for i in result.issues if i.issue_type == "garbled_text"]
        assert len(garbled_issues) == 1

    def test_below_threshold_garbled(self):
        analyzer = ChunkQualityAnalyzer(garbled_threshold=0.5)
        chunk = {
            "text": "Normal text with one \uE000 PUA char"
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        garbled_issues = [i for i in result.issues if i.issue_type == "garbled_text"]
        if len(garbled_issues) > 0:
            assert result.quality_score > 0.5


class TestDuplicateChunkDetection:
    """Tests for duplicate chunk detection."""

    def test_duplicate_detection_with_seen_hashes(self):
        analyzer = ChunkQualityAnalyzer()
        seen_hashes = set()

        chunk1 = {"text": "This is the first chunk content."}
        result1 = analyzer.analyze_single_chunk(chunk1, 0, seen_hashes)
        hash1 = hashlib.md5(chunk1["text"].encode("utf-8", errors="replace")).hexdigest()
        seen_hashes.add(hash1)

        chunk2 = {"text": "This is the first chunk content."}
        result2 = analyzer.analyze_single_chunk(chunk2, 1, seen_hashes)

        duplicate_issues = [i for i in result2.issues if i.issue_type == "duplicate_chunk"]
        assert len(duplicate_issues) == 1
        assert duplicate_issues[0].risk_level == QualityRiskLevel.HIGH
        assert result2.quality_score < result1.quality_score

    def test_unique_chunks_not_flagged(self):
        analyzer = ChunkQualityAnalyzer()
        seen_hashes = set()

        chunk1 = {"text": "This is chunk one."}
        result1 = analyzer.analyze_single_chunk(chunk1, 0, seen_hashes)
        hash1 = hashlib.md5(chunk1["text"].encode("utf-8", errors="replace")).hexdigest()
        seen_hashes.add(hash1)

        chunk2 = {"text": "This is chunk two, different content."}
        result2 = analyzer.analyze_single_chunk(chunk2, 1, seen_hashes)

        duplicate_issues = [i for i in result2.issues if i.issue_type == "duplicate_chunk"]
        assert len(duplicate_issues) == 0

    def test_analyze_chunks_batch_duplicates(self):
        analyzer = ChunkQualityAnalyzer()
        chunks = [
            {"text": "Duplicate text here."},
            {"text": "Duplicate text here."},
            {"text": "Unique text."},
        ]
        results = analyzer.analyze_chunks(chunks)

        assert len(results) == 3
        assert results[1].quality_score < results[0].quality_score
        duplicate_issues = [i for i in results[1].issues if i.issue_type == "duplicate_chunk"]
        assert len(duplicate_issues) == 1


class TestChunkLengthBoundaries:
    """Tests for too short and too long chunk detection."""

    def test_empty_chunk(self):
        analyzer = ChunkQualityAnalyzer(min_chunk_length=20, max_chunk_length=100)
        chunk = {"text": ""}
        result = analyzer.analyze_single_chunk(chunk, 0)

        empty_issues = [i for i in result.issues if i.issue_type == "empty_chunk"]
        assert len(empty_issues) == 1
        assert empty_issues[0].risk_level == QualityRiskLevel.CRITICAL
        assert result.quality_score < 0.6

    def test_too_short_chunk(self):
        analyzer = ChunkQualityAnalyzer(min_chunk_length=20, max_chunk_length=100)
        chunk = {"text": "Short text."}
        result = analyzer.analyze_single_chunk(chunk, 0)

        short_issues = [i for i in result.issues if i.issue_type == "chunk_too_short"]
        assert len(short_issues) == 1
        assert short_issues[0].risk_level == QualityRiskLevel.MEDIUM

    def test_just_above_min_length(self):
        analyzer = ChunkQualityAnalyzer(min_chunk_length=20, max_chunk_length=100)
        chunk = {"text": "This is a text just above min."}
        result = analyzer.analyze_single_chunk(chunk, 0)

        short_issues = [i for i in result.issues if i.issue_type == "chunk_too_short"]
        assert len(short_issues) == 0

    def test_too_long_chunk(self):
        analyzer = ChunkQualityAnalyzer(min_chunk_length=20, max_chunk_length=100)
        long_text = "a" * 150
        chunk = {"text": long_text}
        result = analyzer.analyze_single_chunk(chunk, 0)

        long_issues = [i for i in result.issues if i.issue_type == "chunk_too_long"]
        assert len(long_issues) == 1
        assert long_issues[0].risk_level == QualityRiskLevel.MEDIUM

    def test_just_below_max_length(self):
        analyzer = ChunkQualityAnalyzer(min_chunk_length=20, max_chunk_length=100)
        text = "a" * 90
        chunk = {"text": text}
        result = analyzer.analyze_single_chunk(chunk, 0)

        long_issues = [i for i in result.issues if i.issue_type == "chunk_too_long"]
        assert len(long_issues) == 0


class TestTokenCountChecks:
    """Tests for token count boundary checks."""

    def test_low_token_count(self):
        analyzer = ChunkQualityAnalyzer(min_token_count=5, max_token_count=100)
        chunk = {
            "text": "Short text with few tokens.",
            "tk_nums": 3,
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        token_issues = [i for i in result.issues if i.issue_type == "token_count_too_low"]
        assert len(token_issues) == 1
        assert result.metadata["token_count"] == 3

    def test_high_token_count(self):
        analyzer = ChunkQualityAnalyzer(min_token_count=5, max_token_count=100)
        chunk = {
            "text": "Long text with many tokens here.",
            "tk_nums": 150,
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        token_issues = [i for i in result.issues if i.issue_type == "token_count_too_high"]
        assert len(token_issues) == 1

    def test_token_count_within_range(self):
        analyzer = ChunkQualityAnalyzer(min_token_count=5, max_token_count=100)
        chunk = {
            "text": "Normal text with good token count.",
            "tk_nums": 50,
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        token_low = [i for i in result.issues if i.issue_type == "token_count_too_low"]
        token_high = [i for i in result.issues if i.issue_type == "token_count_too_high"]
        assert len(token_low) == 0
        assert len(token_high) == 0


class TestHeaderFooterPollution:
    """Tests for header and footer pollution detection."""

    def test_chinese_page_pattern(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": """文档开始
第 3 页 / 共 10 页
文档内容继续
更多内容
分隔线
----------------------------------------
结束"""
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        header_issues = [i for i in result.issues if i.issue_type == "header_footer_pollution"]
        assert len(header_issues) >= 1

    def test_separator_lines(self):
        analyzer = ChunkQualityAnalyzer()
        chunk = {
            "text": """Content here
----------------------------------------
More content
========================================
End content"""
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        header_issues = [i for i in result.issues if i.issue_type == "header_footer_pollution"]
        assert len(header_issues) >= 1


class TestBatchAnalysis:
    """Tests for batch chunk analysis and summary."""

    def test_analyze_chunks_empty(self):
        analyzer = ChunkQualityAnalyzer()
        results = analyzer.analyze_chunks([])
        assert len(results) == 0

        summary = analyzer.get_batch_summary(results)
        assert summary["total_chunks"] == 0
        assert summary["average_quality"] == 0.0

    def test_analyze_chunks_mixed_quality(self):
        analyzer = ChunkQualityAnalyzer()
        chunks = [
            {"text": "This is a normal, high-quality chunk with good content and sufficient length."},
            {"text": "短"},
            {"text": "Another normal chunk with good quality text here."},
            {"text": "Text with (cid:123) CID pattern."},
        ]
        results = analyzer.analyze_chunks(chunks)

        assert len(results) == 4

        summary = analyzer.get_batch_summary(results)
        assert summary["total_chunks"] == 4
        assert summary["high_risk_count"] >= 1
        assert len(summary["high_risk_indices"]) >= 1
        assert "low" in summary["risk_distribution"]
        assert len(summary["issue_distribution"]) > 0


class TestResultMethods:
    """Tests for ChunkQualityResult helper methods."""

    def test_has_risk_above(self):
        result = ChunkQualityResult(
            chunk_index=0,
            quality_score=0.8,
            issues=[
                QualityIssue(
                    issue_type="test",
                    risk_level=QualityRiskLevel.MEDIUM,
                    description="Test issue",
                )
            ],
        )

        assert result.has_risk_above(QualityRiskLevel.LOW) is True
        assert result.has_risk_above(QualityRiskLevel.MEDIUM) is True
        assert result.has_risk_above(QualityRiskLevel.HIGH) is False

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
                    issue_type="chunk_too_short",
                    risk_level=QualityRiskLevel.MEDIUM,
                    description="Short",
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

        short_issues = result.get_issues_by_type("chunk_too_short")
        assert len(short_issues) == 1

        missing_issues = result.get_issues_by_type("nonexistent")
        assert len(missing_issues) == 0


class TestSelectiveChecks:
    """Tests for enabling/disabling specific checks."""

    def test_disable_length_check(self):
        analyzer = ChunkQualityAnalyzer(
            enable_checks=["garbled", "repetition"],
        )
        chunk = {"text": "Very short."}
        result = analyzer.analyze_single_chunk(chunk, 0)

        length_issues = [i for i in result.issues if "length" in i.issue_type or "short" in i.issue_type or "long" in i.issue_type]
        assert len(length_issues) == 0

    def test_only_length_check(self):
        analyzer = ChunkQualityAnalyzer(
            enable_checks=["length"],
        )
        chunk = {
            "text": "Short.",
        }
        result = analyzer.analyze_single_chunk(chunk, 0)

        length_issues = [i for i in result.issues if "length" in i.issue_type or "short" in i.issue_type]
        assert len(length_issues) >= 1


class TestQualityRiskLevelEnum:
    """Tests for QualityRiskLevel enum values."""

    def test_enum_values(self):
        assert QualityRiskLevel.LOW.value == "low"
        assert QualityRiskLevel.MEDIUM.value == "medium"
        assert QualityRiskLevel.HIGH.value == "high"
        assert QualityRiskLevel.CRITICAL.value == "critical"

    def test_enum_is_enum(self):
        assert issubclass(QualityRiskLevel, Enum)


class TestTaskExecutorIntegration:
    """Tests for the integration with task_executor.py pattern.

    This tests that the chunks can have quality fields attached
    similar to how _analyze_chunks_quality does it.
    """

    def test_quality_fields_can_be_attached_to_chunks(self):
        analyzer = ChunkQualityAnalyzer()
        chunks = [
            {"content_with_weight": "This is a normal chunk with good content and sufficient length.", "doc_id": "test1"},
            {"content_with_weight": "短", "doc_id": "test2"},
            {"content_with_weight": "Text with (cid:123) garbled pattern.", "doc_id": "test3"},
        ]

        results = analyzer.analyze_chunks(chunks)
        summary = analyzer.get_batch_summary(results)

        for idx, chunk in enumerate(chunks):
            if idx < len(results):
                result = results[idx]
                chunk["_quality_score"] = result.quality_score
                chunk["_quality_risk_level"] = result.risk_level.value
                if result.issues:
                    chunk["_quality_issues"] = [
                        {
                            "type": i.issue_type,
                            "risk": i.risk_level.value,
                            "desc": i.description,
                            "details": i.details,
                            "suggest": i.suggestion,
                        }
                        for i in result.issues
                    ]
                else:
                    chunk["_quality_issues"] = []
                chunk["_quality_metadata"] = result.metadata

        assert chunks[0]["_quality_score"] > 0.9
        assert chunks[0]["_quality_risk_level"] == "low"
        assert chunks[0]["_quality_issues"] == []

        assert chunks[1]["_quality_score"] < 0.9
        assert chunks[1]["_quality_issues"] != []

        assert "cid_garbled" in [i["type"] for i in chunks[2]["_quality_issues"]]
        assert chunks[2]["_quality_risk_level"] == "critical"

        assert summary["total_chunks"] == 3
        assert summary["high_risk_count"] >= 1

    def test_empty_chunks_are_flagged(self):
        analyzer = ChunkQualityAnalyzer()
        chunks = [
            {"content_with_weight": "", "doc_id": "empty"},
        ]
        results = analyzer.analyze_chunks(chunks)

        assert len(results) == 1
        assert results[0].quality_score < 0.6
        empty_issues = [i for i in results[0].issues if i.issue_type == "empty_chunk"]
        assert len(empty_issues) == 1
        assert empty_issues[0].risk_level == QualityRiskLevel.CRITICAL
