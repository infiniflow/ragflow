#!/usr/bin/env python3
"""Standalone test script for Chunk Quality Analyzer.

This script tests the analyzer module directly without depending on
the full RAGFlow infrastructure.
"""

import hashlib
import re
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Dict, List, Optional, Set


class QualityRiskLevel(Enum):
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"


@dataclass
class QualityIssue:
    issue_type: str
    risk_level: QualityRiskLevel
    description: str
    details: Dict[str, Any] = field(default_factory=dict)
    suggestion: str = ""


@dataclass
class ChunkQualityResult:
    chunk_index: int
    quality_score: float
    issues: List[QualityIssue] = field(default_factory=list)
    risk_level: QualityRiskLevel = QualityRiskLevel.LOW
    metadata: Dict[str, Any] = field(default_factory=dict)

    def has_risk_above(self, level: QualityRiskLevel) -> bool:
        level_order = [QualityRiskLevel.LOW, QualityRiskLevel.MEDIUM,
                       QualityRiskLevel.HIGH, QualityRiskLevel.CRITICAL]
        target_idx = level_order.index(level)
        for issue in self.issues:
            if level_order.index(issue.risk_level) >= target_idx:
                return True
        return False

    def get_issues_by_type(self, issue_type: str) -> List[QualityIssue]:
        return [i for i in self.issues if i.issue_type == issue_type]


class ChunkQualityAnalyzer:
    DEFAULT_MIN_CHUNK_LENGTH = 20
    DEFAULT_MAX_CHUNK_LENGTH = 8000
    DEFAULT_MIN_TOKEN_COUNT = 10
    DEFAULT_MAX_TOKEN_COUNT = 2000
    DEFAULT_GARBLED_THRESHOLD = 0.3
    DEFAULT_HEADER_FOOTER_PATTERNS = [
        r"第\s*\d+\s*页\s*/\s*共\s*\d+\s*页",
        r"Page\s*\d+\s*of\s*\d+",
        r"^---+.*---+$",
        r"^[-=_]{3,}$",
        r"Confidential|Private|保密|机密",
    ]

    def __init__(
        self,
        min_chunk_length: Optional[int] = None,
        max_chunk_length: Optional[int] = None,
        min_token_count: Optional[int] = None,
        max_token_count: Optional[int] = None,
        garbled_threshold: Optional[float] = None,
        header_footer_patterns: Optional[List[str]] = None,
        enable_checks: Optional[List[str]] = None,
    ):
        self.min_chunk_length = min_chunk_length or self.DEFAULT_MIN_CHUNK_LENGTH
        self.max_chunk_length = max_chunk_length or self.DEFAULT_MAX_CHUNK_LENGTH
        self.min_token_count = min_token_count or self.DEFAULT_MIN_TOKEN_COUNT
        self.max_token_count = max_token_count or self.DEFAULT_MAX_TOKEN_COUNT
        self.garbled_threshold = garbled_threshold or self.DEFAULT_GARBLED_THRESHOLD
        self.header_footer_patterns = header_footer_patterns or self.DEFAULT_HEADER_FOOTER_PATTERNS
        self.enable_checks = enable_checks or [
            "length", "token_count", "repetition", "garbled",
            "missing_title", "table_break", "header_footer_pollution"
        ]

    def analyze_single_chunk(
        self,
        chunk: Dict[str, Any],
        chunk_index: int = 0,
        seen_hashes: Optional[Set[str]] = None,
    ) -> ChunkQualityResult:
        text = chunk.get("text", "") or chunk.get("content_with_weight", "")
        if not isinstance(text, str):
            text = str(text)

        result = ChunkQualityResult(
            chunk_index=chunk_index,
            quality_score=1.0,
            metadata={
                "text_length": len(text),
                "doc_type": chunk.get("doc_type_kwd", "text"),
                "chunk_type": chunk.get("ck_type", "text"),
            }
        )

        if "length" in self.enable_checks:
            self._check_length(result, text)

        if "token_count" in self.enable_checks:
            self._check_token_count(result, chunk)

        if "repetition" in self.enable_checks and seen_hashes is not None:
            self._check_repetition(result, text, seen_hashes)

        if "garbled" in self.enable_checks:
            self._check_garbled(result, text)

        if "missing_title" in self.enable_checks:
            self._check_missing_title(result, chunk, text)

        if "table_break" in self.enable_checks:
            self._check_table_break(result, chunk, text)

        if "header_footer_pollution" in self.enable_checks:
            self._check_header_footer_pollution(result, text)

        if result.issues:
            max_risk = max(
                [QualityRiskLevel.LOW] +
                [i.risk_level for i in result.issues],
                key=lambda l: [QualityRiskLevel.LOW, QualityRiskLevel.MEDIUM,
                               QualityRiskLevel.HIGH, QualityRiskLevel.CRITICAL].index(l)
            )
            result.risk_level = max_risk

        return result

    def analyze_chunks(
        self,
        chunks: List[Dict[str, Any]],
    ) -> List[ChunkQualityResult]:
        seen_hashes: Set[str] = set()
        results: List[ChunkQualityResult] = []

        for idx, chunk in enumerate(chunks):
            result = self.analyze_single_chunk(chunk, idx, seen_hashes)
            results.append(result)

            if "repetition" in self.enable_checks:
                text = chunk.get("text", "") or chunk.get("content_with_weight", "")
                if text:
                    content_hash = hashlib.md5(str(text).encode("utf-8", errors="replace")).hexdigest()
                    seen_hashes.add(content_hash)

        return results

    def get_batch_summary(
        self,
        results: List[ChunkQualityResult],
    ) -> Dict[str, Any]:
        if not results:
            return {
                "total_chunks": 0,
                "average_quality": 0.0,
                "risk_distribution": {},
                "issue_distribution": {},
            }

        total = len(results)
        avg_score = sum(r.quality_score for r in results) / total

        risk_dist: Dict[str, int] = {}
        issue_dist: Dict[str, int] = {}

        for result in results:
            risk_level = result.risk_level.value
            risk_dist[risk_level] = risk_dist.get(risk_level, 0) + 1

            for issue in result.issues:
                issue_type = issue.issue_type
                issue_dist[issue_type] = issue_dist.get(issue_type, 0) + 1

        high_risk_chunks = [
            r for r in results
            if r.has_risk_above(QualityRiskLevel.HIGH)
        ]

        return {
            "total_chunks": total,
            "average_quality": round(avg_score, 4),
            "risk_distribution": risk_dist,
            "issue_distribution": issue_dist,
            "high_risk_count": len(high_risk_chunks),
            "high_risk_indices": [r.chunk_index for r in high_risk_chunks],
        }

    def _check_length(
        self,
        result: ChunkQualityResult,
        text: str,
    ) -> None:
        text_len = len(text)

        if text_len < self.min_chunk_length:
            if text_len == 0:
                result.quality_score -= 0.5
                result.issues.append(QualityIssue(
                    issue_type="empty_chunk",
                    risk_level=QualityRiskLevel.CRITICAL,
                    description=f"Chunk is empty",
                    details={"length": 0, "threshold": self.min_chunk_length},
                    suggestion="Check parsing process for empty output",
                ))
            else:
                score_penalty = (self.min_chunk_length - text_len) / self.min_chunk_length * 0.3
                result.quality_score = max(0.1, result.quality_score - score_penalty)
                result.issues.append(QualityIssue(
                    issue_type="chunk_too_short",
                    risk_level=QualityRiskLevel.MEDIUM,
                    description=f"Chunk text length ({text_len}) below threshold ({self.min_chunk_length})",
                    details={"length": text_len, "threshold": self.min_chunk_length},
                    suggestion="Consider increasing chunk size or merging with adjacent chunks",
                ))

        elif text_len > self.max_chunk_length:
            score_penalty = (text_len - self.max_chunk_length) / text_len * 0.3
            result.quality_score = max(0.1, result.quality_score - score_penalty)
            result.issues.append(QualityIssue(
                issue_type="chunk_too_long",
                risk_level=QualityRiskLevel.MEDIUM,
                description=f"Chunk text length ({text_len}) exceeds threshold ({self.max_chunk_length})",
                details={"length": text_len, "threshold": self.max_chunk_length},
                suggestion="Consider reducing chunk size or splitting the chunk",
            ))

    def _check_token_count(
        self,
        result: ChunkQualityResult,
        chunk: Dict[str, Any],
    ) -> None:
        tk_nums = chunk.get("tk_nums")
        if tk_nums is None:
            return

        if not isinstance(tk_nums, int):
            try:
                tk_nums = int(tk_nums)
            except (ValueError, TypeError):
                return

        if tk_nums < self.min_token_count:
            score_penalty = (self.min_token_count - tk_nums) / self.min_token_count * 0.2
            result.quality_score = max(0.1, result.quality_score - score_penalty)
            result.issues.append(QualityIssue(
                issue_type="token_count_too_low",
                risk_level=QualityRiskLevel.LOW,
                description=f"Token count ({tk_nums}) below threshold ({self.min_token_count})",
                details={"token_count": tk_nums, "threshold": self.min_token_count},
                suggestion="Chunk may contain insufficient semantic information",
            ))

        elif tk_nums > self.max_token_count:
            score_penalty = (tk_nums - self.max_token_count) / tk_nums * 0.25
            result.quality_score = max(0.1, result.quality_score - score_penalty)
            result.issues.append(QualityIssue(
                issue_type="token_count_too_high",
                risk_level=QualityRiskLevel.MEDIUM,
                description=f"Token count ({tk_nums}) exceeds threshold ({self.max_token_count})",
                details={"token_count": tk_nums, "threshold": self.max_token_count},
                suggestion="Large chunks may reduce retrieval precision",
            ))

        result.metadata["token_count"] = tk_nums

    def _check_repetition(
        self,
        result: ChunkQualityResult,
        text: str,
        seen_hashes: Set[str],
    ) -> None:
        if not text or not seen_hashes:
            return

        content_hash = hashlib.md5(text.encode("utf-8", errors="replace")).hexdigest()

        if content_hash in seen_hashes:
            result.quality_score -= 0.4
            result.issues.append(QualityIssue(
                issue_type="duplicate_chunk",
                risk_level=QualityRiskLevel.HIGH,
                description="Chunk content is identical to a previous chunk",
                details={"content_hash": content_hash},
                suggestion="Review document parsing and chunking strategy to avoid duplicates",
            ))

    def _check_garbled(
        self,
        result: ChunkQualityResult,
        text: str,
    ) -> None:
        if not text:
            return

        garbled_chars = 0
        total_chars = 0
        cid_match = re.search(r"\(cid\s*:\s*\d+\s*\)", text)

        if cid_match:
            result.quality_score -= 0.5
            result.issues.append(QualityIssue(
                issue_type="cid_garbled",
                risk_level=QualityRiskLevel.CRITICAL,
                description="Detected CID pattern indicating font encoding issues",
                details={"pattern": cid_match.group()},
                suggestion="Check PDF font encoding or use a different parsing method",
            ))
            return

        for ch in text:
            if not ch.isspace():
                total_chars += 1
                if self._is_garbled_char(ch):
                    garbled_chars += 1

        if total_chars > 0:
            garbled_ratio = garbled_chars / total_chars

            if garbled_ratio >= self.garbled_threshold:
                risk_level = QualityRiskLevel.HIGH if garbled_ratio >= 0.5 else QualityRiskLevel.MEDIUM
                result.quality_score = max(0.1, result.quality_score - garbled_ratio * 0.6)
                result.issues.append(QualityIssue(
                    issue_type="garbled_text",
                    risk_level=risk_level,
                    description=f"Garbled characters detected: {garbled_ratio*100:.1f}% of content",
                    details={
                        "garbled_count": garbled_chars,
                        "total_count": total_chars,
                        "ratio": garbled_ratio,
                    },
                    suggestion="Check document encoding or use OCR-based parsing",
                ))

            result.metadata["garbled_ratio"] = garbled_ratio

    def _is_garbled_char(self, ch: str) -> bool:
        if not ch:
            return False

        code = ord(ch)

        if 0xE000 <= code <= 0xF8FF:
            return True

        if 0xF0000 <= code <= 0x10FFFF:
            return True

        if code == 0xFFFD:
            return True

        if 0x00 <= code <= 0x1F and code not in (0x09, 0x0A, 0x0D):
            return True

        if 0x80 <= code <= 0x9F:
            return True

        return False

    def _check_missing_title(
        self,
        result: ChunkQualityResult,
        chunk: Dict[str, Any],
        text: str,
    ) -> None:
        doc_type = chunk.get("doc_type_kwd", "text")
        chunk_type = chunk.get("ck_type", "text")

        if doc_type == "table" or chunk_type == "table":
            if not self._has_table_header(text):
                result.quality_score -= 0.15
                result.issues.append(QualityIssue(
                    issue_type="missing_table_header",
                    risk_level=QualityRiskLevel.LOW,
                    description="Table chunk may be missing header context",
                    details={},
                    suggestion="Consider enabling table context or checking table extraction",
                ))
        elif doc_type == "text" and len(text) > 100:
            has_header_marker = self._has_header_indicator(text)
            result.metadata["has_header_marker"] = has_header_marker

    def _has_table_header(self, text: str) -> bool:
        if "---" in text and "|" in text:
            return True

        html_table_patterns = [
            r"<thead>",
            r"<th>",
            r"colspan=",
            r"rowspan=",
        ]
        for pattern in html_table_patterns:
            if re.search(pattern, text, re.IGNORECASE):
                return True

        return False

    def _has_header_indicator(self, text: str) -> bool:
        lines = text.strip().split("\n")
        if not lines:
            return False

        first_line = lines[0].strip()

        if re.match(r"^#{1,6}\s+\w", first_line):
            return True

        if re.match(r"^[一二三四五六七八九十]+[、\.\s]", first_line):
            return True

        if re.match(r"^\d+[\.\)]\s+\w", first_line):
            return True

        if re.match(r"^[第章节篇]\s*\d+", first_line):
            return True

        return False

    def _check_table_break(
        self,
        result: ChunkQualityResult,
        chunk: Dict[str, Any],
        text: str,
    ) -> None:
        doc_type = chunk.get("doc_type_kwd", "text")
        chunk_type = chunk.get("ck_type", "text")

        if doc_type != "table" and chunk_type != "table":
            return

        is_incomplete = False
        break_details = {}

        table_tags = ["<table>", "<tr>", "<td>", "<th>"]
        for tag in table_tags:
            if text.count(tag) != text.count(tag.replace("<", "</")):
                is_incomplete = True
                break_details["unclosed_tags"] = True
                break

        pipe_count = text.count("|")
        if "---" in text and "|" in text:
            if pipe_count < 4:
                is_incomplete = True
                break_details["insufficient_pipes"] = True

        row_patterns = [
            r"<tr[^>]*>",
            r"^\|.*\|$",
        ]
        rows_found = 0
        for pattern in row_patterns:
            rows_found += len(re.findall(pattern, text, re.MULTILINE))

        if rows_found <= 1:
            is_incomplete = True
            break_details["few_rows"] = rows_found

        if is_incomplete:
            result.quality_score -= 0.25
            result.issues.append(QualityIssue(
                issue_type="table_break",
                risk_level=QualityRiskLevel.HIGH,
                description="Table appears to be broken or incomplete",
                details=break_details,
                suggestion="Check table extraction settings or increase table context size",
            ))

    def _check_header_footer_pollution(
        self,
        result: ChunkQualityResult,
        text: str,
    ) -> None:
        if not text:
            return

        lines = text.split("\n")
        if len(lines) < 3:
            return

        pollution_count = 0
        pollution_patterns = []

        check_lines = lines[:3] + lines[-3:]
        for line in check_lines:
            line_stripped = line.strip()
            if not line_stripped:
                continue

            for pattern in self.header_footer_patterns:
                if re.search(pattern, line_stripped, re.IGNORECASE):
                    pollution_count += 1
                    pollution_patterns.append(line_stripped[:100])
                    break

        page_number_patterns = [
            r"^-?\s*\d+\s*-$",
            r"^\d+\s*$",
            r"^Page\s+\d+.*$",
        ]

        for line in check_lines:
            line_stripped = line.strip()
            for pattern in page_number_patterns:
                if re.match(pattern, line_stripped, re.IGNORECASE):
                    if len(line_stripped) < 20:
                        pollution_count += 1
                        pollution_patterns.append(f"page_number: {line_stripped[:50]}")
                    break

        if pollution_count >= 2:
            result.quality_score -= 0.2
            result.issues.append(QualityIssue(
                issue_type="header_footer_pollution",
                risk_level=QualityRiskLevel.MEDIUM,
                description=f"Detected possible header/footer pollution ({pollution_count} patterns)",
                details={
                    "count": pollution_count,
                    "patterns": pollution_patterns[:5],
                },
                suggestion="Check if TOC removal is enabled or adjust parsing settings",
            ))


def run_tests():
    """Run all unit tests for ChunkQualityAnalyzer."""
    print("=" * 60)
    print("Running Chunk Quality Analyzer Tests")
    print("=" * 60)

    test_passed = 0
    test_failed = 0

    def test(name, condition):
        nonlocal test_passed, test_failed
        if condition:
            print(f"  [PASS] {name}")
            test_passed += 1
        else:
            print(f"  [FAIL] {name}")
            test_failed += 1

    print("\n--- Test 1: Normal text chunks ---")
    analyzer = ChunkQualityAnalyzer()

    normal_chinese = {
        "text": "这是一段正常的中文文本内容，用于测试质量分析器的基本功能。"
                "这段文本包含足够的长度和丰富的内容，应该被判定为高质量的chunk。"
                "它没有乱码，没有重复，也没有任何格式问题。"
    }
    result = analyzer.analyze_single_chunk(normal_chinese, 0)
    test("Normal Chinese text - score > 0.9", result.quality_score > 0.9)
    test("Normal Chinese text - no issues", len(result.issues) == 0)
    test("Normal Chinese text - LOW risk", result.risk_level == QualityRiskLevel.LOW)

    normal_english = {
        "text": "This is a normal English text chunk with sufficient length. "
                "It contains multiple sentences and provides rich semantic content. "
                "The quality analyzer should mark this as a high-quality chunk."
    }
    result2 = analyzer.analyze_single_chunk(normal_english, 1)
    test("Normal English text - score > 0.9", result2.quality_score > 0.9)
    test("Normal English text - no issues", len(result2.issues) == 0)

    print("\n--- Test 2: Table chunks ---")

    complete_table = {
        "text": "<table><thead><tr><th>Name</th><th>Age</th></tr></thead>"
                "<tbody><tr><td>John</td><td>25</td></tr></tbody></table>",
        "doc_type_kwd": "table",
        "ck_type": "table",
    }
    result3 = analyzer.analyze_single_chunk(complete_table, 0)
    table_break_issues = [i for i in result3.issues if i.issue_type == "table_break"]
    test("Complete HTML table - no table_break issues", len(table_break_issues) == 0)

    broken_table = {
        "text": "<table><thead><tr><th>Name</th>",
        "doc_type_kwd": "table",
        "ck_type": "table",
    }
    result4 = analyzer.analyze_single_chunk(broken_table, 0)
    table_break_issues = [i for i in result4.issues if i.issue_type == "table_break"]
    test("Broken table - has table_break issue", len(table_break_issues) >= 1)

    print("\n--- Test 3: Garbled text detection ---")

    cid_text = {"text": "This is some text with (cid:123) CID patterns."}
    result5 = analyzer.analyze_single_chunk(cid_text, 0)
    cid_issues = [i for i in result5.issues if i.issue_type == "cid_garbled"]
    test("CID pattern - detected cid_garbled issue", len(cid_issues) == 1)
    test("CID pattern - CRITICAL risk", cid_issues[0].risk_level == QualityRiskLevel.CRITICAL)

    pua_text = {"text": "\uE000\uE001\uE002\uE003\uE004\uE005\uE006\uE007\uE008 Normal"}
    result6 = analyzer.analyze_single_chunk(pua_text, 0)
    garbled_issues = [i for i in result6.issues if i.issue_type == "garbled_text"]
    test("PUA chars - detected garbled_text issue", len(garbled_issues) == 1)

    print("\n--- Test 4: Duplicate chunk detection ---")

    seen_hashes = set()
    chunk1 = {"text": "Duplicate content here."}
    result7 = analyzer.analyze_single_chunk(chunk1, 0, seen_hashes)
    import hashlib
    hash1 = hashlib.md5(chunk1["text"].encode("utf-8", errors="replace")).hexdigest()
    seen_hashes.add(hash1)

    chunk2 = {"text": "Duplicate content here."}
    result8 = analyzer.analyze_single_chunk(chunk2, 1, seen_hashes)
    duplicate_issues = [i for i in result8.issues if i.issue_type == "duplicate_chunk"]
    test("Duplicate - detected duplicate_chunk issue", len(duplicate_issues) == 1)
    test("Duplicate - score lower than original", result8.quality_score < result7.quality_score)

    print("\n--- Test 5: Chunk length boundaries ---")

    analyzer2 = ChunkQualityAnalyzer(min_chunk_length=20, max_chunk_length=100)

    empty_chunk = {"text": ""}
    result9 = analyzer2.analyze_single_chunk(empty_chunk, 0)
    empty_issues = [i for i in result9.issues if i.issue_type == "empty_chunk"]
    test("Empty chunk - detected empty_chunk issue", len(empty_issues) == 1)
    test("Empty chunk - CRITICAL risk", empty_issues[0].risk_level == QualityRiskLevel.CRITICAL)

    short_chunk = {"text": "Short text."}
    result10 = analyzer2.analyze_single_chunk(short_chunk, 0)
    short_issues = [i for i in result10.issues if i.issue_type == "chunk_too_short"]
    test("Short chunk - detected chunk_too_short issue", len(short_issues) == 1)

    long_chunk = {"text": "a" * 150}
    result11 = analyzer2.analyze_single_chunk(long_chunk, 0)
    long_issues = [i for i in result11.issues if i.issue_type == "chunk_too_long"]
    test("Long chunk - detected chunk_too_long issue", len(long_issues) == 1)

    print("\n--- Test 6: Token count checks ---")

    analyzer3 = ChunkQualityAnalyzer(min_token_count=5, max_token_count=100)

    low_token = {"text": "Short text.", "tk_nums": 3}
    result12 = analyzer3.analyze_single_chunk(low_token, 0)
    token_low_issues = [i for i in result12.issues if i.issue_type == "token_count_too_low"]
    test("Low token count - detected", len(token_low_issues) == 1)

    high_token = {"text": "Long text.", "tk_nums": 150}
    result13 = analyzer3.analyze_single_chunk(high_token, 0)
    token_high_issues = [i for i in result13.issues if i.issue_type == "token_count_too_high"]
    test("High token count - detected", len(token_high_issues) == 1)

    print("\n--- Test 7: Header/footer pollution ---")

    header_text = {
        "text": """Some document content here.

第 3 页 / 共 10 页
More content continues.

----------------------------------------
End content"""
    }
    result14 = analyzer.analyze_single_chunk(header_text, 0)
    header_issues = [i for i in result14.issues if i.issue_type == "header_footer_pollution"]
    test("Header/footer patterns - detected", len(header_issues) >= 1)

    print("\n--- Test 8: Batch analysis ---")

    chunks_batch = [
        {"text": "This is a normal, high-quality chunk with good content."},
        {"text": "短"},
        {"text": "Another normal chunk with good quality text here."},
        {"text": "Text with (cid:123) CID pattern."},
    ]
    results_batch = analyzer.analyze_chunks(chunks_batch)
    test("Batch analysis - correct count", len(results_batch) == 4)

    summary = analyzer.get_batch_summary(results_batch)
    test("Batch summary - total chunks", summary["total_chunks"] == 4)
    test("Batch summary - high risk count", summary["high_risk_count"] >= 1)
    test("Batch summary - issue distribution", len(summary["issue_distribution"]) > 0)

    print("\n--- Test 9: Result helper methods ---")

    result15 = ChunkQualityResult(
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
    test("has_risk_above LOW", result15.has_risk_above(QualityRiskLevel.LOW) is True)
    test("has_risk_above MEDIUM", result15.has_risk_above(QualityRiskLevel.MEDIUM) is True)
    test("has_risk_above HIGH", result15.has_risk_above(QualityRiskLevel.HIGH) is False)

    print("\n--- Test 10: Selective checks ---")

    analyzer4 = ChunkQualityAnalyzer(enable_checks=["garbled"])
    short_text = {"text": "Very short."}
    result16 = analyzer4.analyze_single_chunk(short_text, 0)
    length_issues = [i for i in result16.issues if "length" in i.issue_type or "short" in i.issue_type]
    test("Selective checks - length check disabled", len(length_issues) == 0)

    print("\n" + "=" * 60)
    print(f"Test Results: {test_passed} passed, {test_failed} failed")
    print("=" * 60)

    return test_failed == 0


if __name__ == "__main__":
    success = run_tests()
    exit(0 if success else 1)
