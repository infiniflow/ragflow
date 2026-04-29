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

        mojibake_detection = self._detect_mojibake_pattern(text)
        result.metadata["mojibake_ratio"] = mojibake_detection.get("mojibake_ratio", 0.0)
        result.metadata["replacement_char_count"] = mojibake_detection.get("replacement_char_count", 0)
        result.metadata["extended_latin1_count"] = mojibake_detection.get("extended_latin1_count", 0)
        result.metadata["suspicious_sequences"] = mojibake_detection.get("suspicious_sequences", 0)

        if mojibake_detection["detected"]:
            reason = mojibake_detection.get("reason", "unknown")
            mojibake_ratio = mojibake_detection.get("mojibake_ratio", 0.0)

            if mojibake_ratio >= 0.5 or reason in ["replacement_chars", "suspicious_sequences"]:
                risk_level = QualityRiskLevel.HIGH
                score_penalty = 0.4
            else:
                risk_level = QualityRiskLevel.MEDIUM
                score_penalty = 0.3

            result.quality_score = max(0.1, result.quality_score - score_penalty)
            result.issues.append(QualityIssue(
                issue_type="mojibake_text",
                risk_level=risk_level,
                description=f"Detected possible encoding error/mojibake (reason: {reason})",
                details={
                    "reason": reason,
                    "mojibake_ratio": mojibake_ratio,
                    "replacement_char_count": mojibake_detection.get("replacement_char_count", 0),
                    "extended_latin1_count": mojibake_detection.get("extended_latin1_count", 0),
                    "suspicious_sequences": mojibake_detection.get("suspicious_sequences", 0),
                },
                suggestion="Check document encoding. UTF-8/GBK encoding mismatch is common cause of Chinese mojibake.",
            ))
            return

        for ch in text:
            if not ch.isspace():
                total_chars += 1
                if self._is_garbled_char(ch):
                    garbled_chars += 1

        if total_chars > 0:
            garbled_ratio = garbled_chars / total_chars
            result.metadata["garbled_ratio"] = garbled_ratio

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

    def _is_mojibake_char(self, ch: str) -> bool:
        if not ch:
            return False

        code = ord(ch)

        if 0xC0 <= code <= 0xFF:
            return True

        if code == 0xB0:
            return True

        if 0x2000 <= code <= 0x206F:
            return True

        if 0x2100 <= code <= 0x214F:
            return True

        if code in [0x00A0, 0x00AD]:
            return True

        return False

    def _detect_mojibake_pattern(self, text: str) -> Dict[str, Any]:
        if not text:
            return {"detected": False}

        mojibake_chars = 0
        total_chars = 0
        extended_latin1_count = 0
        replacement_char_count = 0
        control_char_count = 0
        suspicious_sequences = 0

        mojibake_sequences = [
            r"[ÀÁÂÃÄÅÆÇÈÉÊËÌÍÎÏÐÑÒÓÔÕÖ×ØÙÚÛÜÝÞß]{2,}",
            r"[äåæçèéêëìíîïðñòóôõö÷øùúûüýþÿ]{2,}",
            r"[°±²³´µ¶·¹º»¼½¾]{2,}",
            r"[\u2018\u2019\u201C\u201D\u2013\u2014\u2026]{2,}",
            r"�+",
        ]

        long_mojibake_sequences = 0
        for pattern in mojibake_sequences:
            matches = re.findall(pattern, text)
            for match in matches:
                suspicious_sequences += 1
                if len(match) >= 5:
                    long_mojibake_sequences += 1

        for ch in text:
            if ch.isspace():
                continue
            total_chars += 1

            if self._is_mojibake_char(ch):
                mojibake_chars += 1

            code = ord(ch)
            if 0xC0 <= code <= 0xFF:
                extended_latin1_count += 1

            if code == 0xFFFD:
                replacement_char_count += 1

            if self._is_garbled_char(ch):
                control_char_count += 1

        result = {
            "detected": False,
            "mojibake_ratio": 0.0,
            "extended_latin1_count": extended_latin1_count,
            "replacement_char_count": replacement_char_count,
            "control_char_count": control_char_count,
            "suspicious_sequences": suspicious_sequences,
            "long_mojibake_sequences": long_mojibake_sequences,
        }

        if total_chars == 0:
            return result

        mojibake_ratio = mojibake_chars / total_chars
        result["mojibake_ratio"] = mojibake_ratio

        if replacement_char_count >= 1 and replacement_char_count / max(total_chars, 1) > 0.1:
            result["detected"] = True
            result["reason"] = "replacement_chars"

        if long_mojibake_sequences >= 1 or suspicious_sequences >= 2:
            result["detected"] = True
            result["reason"] = "suspicious_sequences"

        if mojibake_ratio >= 0.25 and extended_latin1_count >= 3:
            result["detected"] = True
            result["reason"] = "high_mojibake_ratio"

        if mojibake_ratio >= 0.5:
            result["detected"] = True
            result["reason"] = "very_high_mojibake_ratio"

        return result

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
