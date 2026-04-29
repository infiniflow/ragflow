#!/usr/bin/env python3
"""Direct test script for ChunkQualityAnalyzer using only stdlib.

Enhanced with mojibake detection tests.
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

print(f"Loading analyzer from: {_MODULE_PATH}")
assert os.path.exists(_MODULE_PATH), f"Module not found: {_MODULE_PATH}"

_spec = importlib.util.spec_from_file_location("analyzer", _MODULE_PATH)
_mod = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_mod)

ChunkQualityAnalyzer = _mod.ChunkQualityAnalyzer
ChunkQualityResult = _mod.ChunkQualityResult
QualityIssue = _mod.QualityIssue
QualityRiskLevel = _mod.QualityRiskLevel

print("[OK] Successfully imported ChunkQualityAnalyzer")


def run_tests():
    """Run all tests."""
    tests_passed = 0
    tests_failed = 0

    def test(name, condition):
        nonlocal tests_passed, tests_failed
        if condition:
            print(f"  [PASS] {name}")
            tests_passed += 1
        else:
            print(f"  [FAIL] {name}")
            tests_failed += 1

    print("\n" + "=" * 60)
    print("Test 1: Initialization")
    print("=" * 60)

    analyzer = ChunkQualityAnalyzer()
    test("Default min_chunk_length == 20", analyzer.min_chunk_length == 20)
    test("Default max_chunk_length == 8000", analyzer.max_chunk_length == 8000)
    test("Default garbled_threshold == 0.3", analyzer.garbled_threshold == 0.3)
    test("enable_checks is not empty", len(analyzer.enable_checks) > 0)

    print("\n" + "=" * 60)
    print("Test 2: Normal text chunks")
    print("=" * 60)

    chinese_text = {
        "text": "这是一段正常的中文文本内容，用于测试质量分析器的基本功能。"
                "这段文本包含足够的长度和丰富的内容，应该被判定为高质量的chunk。"
                "它没有乱码，没有重复，也没有任何格式问题。"
    }
    result = analyzer.analyze_single_chunk(chinese_text, 0)
    test("Normal Chinese - score > 0.9", result.quality_score > 0.9)
    test("Normal Chinese - no issues", len(result.issues) == 0)
    test("Normal Chinese - LOW risk", result.risk_level == QualityRiskLevel.LOW)

    english_text = {
        "text": "This is a normal English text chunk with sufficient length. "
                "It contains multiple sentences and provides rich semantic content. "
                "The quality analyzer should mark this as a high-quality chunk."
    }
    result2 = analyzer.analyze_single_chunk(english_text, 1)
    test("Normal English - score > 0.9", result2.quality_score > 0.9)
    test("Normal English - no issues", len(result2.issues) == 0)

    print("\n" + "=" * 60)
    print("Test 3: Length boundaries")
    print("=" * 60)

    analyzer2 = ChunkQualityAnalyzer(min_chunk_length=20, max_chunk_length=100)

    empty = {"text": ""}
    result_empty = analyzer2.analyze_single_chunk(empty, 0)
    empty_issues = [i for i in result_empty.issues if i.issue_type == "empty_chunk"]
    test("Empty chunk - has empty_chunk issue", len(empty_issues) == 1)
    test("Empty chunk - CRITICAL risk", empty_issues[0].risk_level == QualityRiskLevel.CRITICAL)

    short = {"text": "Short text."}
    result_short = analyzer2.analyze_single_chunk(short, 0)
    short_issues = [i for i in result_short.issues if i.issue_type == "chunk_too_short"]
    test("Short chunk - has chunk_too_short issue", len(short_issues) == 1)

    long_text = {"text": "a" * 150}
    result_long = analyzer2.analyze_single_chunk(long_text, 0)
    long_issues = [i for i in result_long.issues if i.issue_type == "chunk_too_long"]
    test("Long chunk - has chunk_too_long issue", len(long_issues) == 1)

    print("\n" + "=" * 60)
    print("Test 4: CID garbled detection")
    print("=" * 60)

    cid_text = {"text": "Text with (cid:123) CID pattern."}
    result_cid = analyzer.analyze_single_chunk(cid_text, 0)
    cid_issues = [i for i in result_cid.issues if i.issue_type == "cid_garbled"]
    test("CID pattern - has cid_garbled issue", len(cid_issues) == 1)
    test("CID pattern - CRITICAL risk", cid_issues[0].risk_level == QualityRiskLevel.CRITICAL)

    cid_text2 = {"text": "Text with (cid : 45) and more (cid:6789)."}
    result_cid2 = analyzer.analyze_single_chunk(cid_text2, 0)
    cid_issues2 = [i for i in result_cid2.issues if i.issue_type == "cid_garbled"]
    test("CID with spaces - detected", len(cid_issues2) == 1)

    print("\n" + "=" * 60)
    print("Test 5: PUA and control character detection")
    print("=" * 60)

    analyzer3 = ChunkQualityAnalyzer(garbled_threshold=0.3)
    pua_text = {"text": "\uE000\uE001\uE002\uE003\uE004\uE005\uE006\uE007\uE008 Normal"}
    result_pua = analyzer3.analyze_single_chunk(pua_text, 0)
    pua_issues = [i for i in result_pua.issues if i.issue_type in ["garbled_text", "mojibake_text"]]
    test("PUA chars - has garbled/mojibake issue", len(pua_issues) >= 1)

    print("\n" + "=" * 60)
    print("Test 6: Mojibake detection - replacement chars")
    print("=" * 60)

    replacement_text = {"text": "Text with \ufffd\ufffd\ufffd replacement chars."}
    result_repl = analyzer.analyze_single_chunk(replacement_text, 0)
    repl_issues = [i for i in result_repl.issues if i.issue_type == "mojibake_text"]
    test("Replacement chars - has mojibake_text issue", len(repl_issues) == 1)
    test("Replacement chars - in metadata", result_repl.metadata.get("replacement_char_count", 0) == 3)

    print("\n" + "=" * 60)
    print("Test 7: Mojibake detection - extended Latin-1")
    print("=" * 60)

    mojibake_text1 = {"text": "ÀÁÂÃÄÅÆÇÈÉÊËÌÍÎÏÐÑÒÓÔÕÖ"}
    result_moj1 = analyzer.analyze_single_chunk(mojibake_text1, 0)
    moj_issues1 = [i for i in result_moj1.issues if i.issue_type == "mojibake_text"]
    test("Extended Latin-1 sequence - has mojibake_text issue", len(moj_issues1) == 1)
    test("Extended Latin-1 count in metadata", result_moj1.metadata.get("extended_latin1_count", 0) > 0)

    mojibake_text2 = {"text": "äåæçèéêëìíîïðñòóôõö÷ø"}
    result_moj2 = analyzer.analyze_single_chunk(mojibake_text2, 0)
    moj_issues2 = [i for i in result_moj2.issues if i.issue_type == "mojibake_text"]
    test("Lowercase extended Latin-1 - has mojibake_text issue", len(moj_issues2) == 1)

    print("\n" + "=" * 60)
    print("Test 8: Mojibake detection - suspicious sequences")
    print("=" * 60)

    mojibake_text3 = {"text": "°±²³´µ¶·¹º»¼½¾ more °±²³"}
    result_moj3 = analyzer.analyze_single_chunk(mojibake_text3, 0)
    moj_issues3 = [i for i in result_moj3.issues if i.issue_type == "mojibake_text"]
    test("Superscript/subscript sequence - has mojibake_text issue", len(moj_issues3) >= 1)

    print("\n" + "=" * 60)
    print("Test 9: Normal text NOT misclassified as mojibake")
    print("=" * 60)

    valid_french = {"text": "Bonjour, je m'appelle Pierre. Je vis à Paris."}
    result_french = analyzer.analyze_single_chunk(valid_french, 0)
    french_issues = [i for i in result_french.issues if i.issue_type == "mojibake_text"]
    test("Valid French - NOT mojibake", len(french_issues) == 0)

    valid_german = {"text": "Guten Tag, ich heiße Maria. Ich wohne in München."}
    result_german = analyzer.analyze_single_chunk(valid_german, 0)
    german_issues = [i for i in result_german.issues if i.issue_type == "mojibake_text"]
    test("Valid German - NOT mojibake", len(german_issues) == 0)

    valid_spanish = {"text": "Hola, me llamo Carlos. Vivo en Madrid."}
    result_spanish = analyzer.analyze_single_chunk(valid_spanish, 0)
    spanish_issues = [i for i in result_spanish.issues if i.issue_type == "mojibake_text"]
    test("Valid Spanish - NOT mojibake", len(spanish_issues) == 0)

    print("\n" + "=" * 60)
    print("Test 10: Duplicate detection")
    print("=" * 60)

    seen_hashes = set()
    chunk1 = {"text": "This is the first chunk content."}
    result1 = analyzer.analyze_single_chunk(chunk1, 0, seen_hashes)
    hash1 = hashlib.md5(chunk1["text"].encode("utf-8", errors="replace")).hexdigest()
    seen_hashes.add(hash1)

    chunk2 = {"text": "This is the first chunk content."}
    result2 = analyzer.analyze_single_chunk(chunk2, 1, seen_hashes)

    dup_issues = [i for i in result2.issues if i.issue_type == "duplicate_chunk"]
    test("Duplicate - has duplicate_chunk issue", len(dup_issues) == 1)
    test("Duplicate - score lower than original", result2.quality_score < result1.quality_score)

    chunk3 = {"text": "Unique content here."}
    result3 = analyzer.analyze_single_chunk(chunk3, 2, seen_hashes)
    dup_issues3 = [i for i in result3.issues if i.issue_type == "duplicate_chunk"]
    test("Unique - no duplicate issue", len(dup_issues3) == 0)

    print("\n" + "=" * 60)
    print("Test 11: Table detection")
    print("=" * 60)

    complete_table = {
        "text": "<table><thead><tr><th>Name</th><th>Age</th></tr></thead>"
                "<tbody><tr><td>John</td><td>25</td></tr></tbody></table>",
        "doc_type_kwd": "table",
        "ck_type": "table",
    }
    result_table = analyzer.analyze_single_chunk(complete_table, 0)
    table_break_issues = [i for i in result_table.issues if i.issue_type == "table_break"]
    test("Complete table - no table_break issue", len(table_break_issues) == 0)

    broken_table = {
        "text": "<table><thead><tr><th>Name</th>",
        "doc_type_kwd": "table",
        "ck_type": "table",
    }
    result_broken = analyzer.analyze_single_chunk(broken_table, 0)
    broken_issues = [i for i in result_broken.issues if i.issue_type == "table_break"]
    test("Broken table - has table_break issue", len(broken_issues) >= 1)

    print("\n" + "=" * 60)
    print("Test 12: Header/Footer pollution")
    print("=" * 60)

    header_text = {
        "text": """文档开始
第 3 页 / 共 10 页
文档内容继续
更多内容
----------------------------------------
结束"""
    }
    result_header = analyzer.analyze_single_chunk(header_text, 0)
    header_issues = [i for i in result_header.issues if i.issue_type == "header_footer_pollution"]
    test("Header/footer patterns - detected", len(header_issues) >= 1)

    print("\n" + "=" * 60)
    print("Test 13: Token count checks")
    print("=" * 60)

    analyzer4 = ChunkQualityAnalyzer(min_token_count=5, max_token_count=100)

    low_token = {"text": "Short.", "tk_nums": 3}
    result_low = analyzer4.analyze_single_chunk(low_token, 0)
    low_issues = [i for i in result_low.issues if i.issue_type == "token_count_too_low"]
    test("Low token count - detected", len(low_issues) == 1)

    high_token = {"text": "Long.", "tk_nums": 150}
    result_high = analyzer4.analyze_single_chunk(high_token, 0)
    high_issues = [i for i in result_high.issues if i.issue_type == "token_count_too_high"]
    test("High token count - detected", len(high_issues) == 1)

    print("\n" + "=" * 60)
    print("Test 14: Batch analysis")
    print("=" * 60)

    batch_chunks = [
        {"text": "This is a normal, high-quality chunk with good content and sufficient length."},
        {"text": "短"},
        {"text": "Another normal chunk with good quality text here."},
        {"text": "Text with (cid:123) CID pattern."},
    ]
    results_batch = analyzer.analyze_chunks(batch_chunks)
    summary = analyzer.get_batch_summary(results_batch)

    test("Batch - total_chunks == 4", summary["total_chunks"] == 4)
    test("Batch - high_risk_count >= 1", summary["high_risk_count"] >= 1)
    test("Batch - issue_distribution not empty", len(summary["issue_distribution"]) > 0)

    print("\n" + "=" * 60)
    print("Test 15: Result helper methods")
    print("=" * 60)

    result_test = ChunkQualityResult(
        chunk_index=0,
        quality_score=0.8,
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
        ],
    )

    test("has_risk_above(LOW) == True", result_test.has_risk_above(QualityRiskLevel.LOW) is True)
    test("has_risk_above(HIGH) == True", result_test.has_risk_above(QualityRiskLevel.HIGH) is True)
    test("has_risk_above(CRITICAL) == False", result_test.has_risk_above(QualityRiskLevel.CRITICAL) is False)

    print("\n" + "=" * 60)
    print("Test 16: Selective checks")
    print("=" * 60)

    analyzer_garbled_only = ChunkQualityAnalyzer(enable_checks=["garbled"])
    short_chunk = {"text": "Very short."}
    result_sel = analyzer_garbled_only.analyze_single_chunk(short_chunk, 0)
    length_issues = [i for i in result_sel.issues if "length" in i.issue_type or "short" in i.issue_type]
    test("Selective checks - length check disabled", len(length_issues) == 0)

    analyzer_length_only = ChunkQualityAnalyzer(enable_checks=["length"])
    result_len = analyzer_length_only.analyze_single_chunk(short_chunk, 0)
    short_issues2 = [i for i in result_len.issues if "length" in i.issue_type or "short" in i.issue_type]
    test("Selective checks - length check enabled", len(short_issues2) >= 1)

    print("\n" + "=" * 60)
    print("Test 17: QualityRiskLevel enum")
    print("=" * 60)

    test("QualityRiskLevel.LOW.value == 'low'", QualityRiskLevel.LOW.value == "low")
    test("QualityRiskLevel.MEDIUM.value == 'medium'", QualityRiskLevel.MEDIUM.value == "medium")
    test("QualityRiskLevel.HIGH.value == 'high'", QualityRiskLevel.HIGH.value == "high")
    test("QualityRiskLevel.CRITICAL.value == 'critical'", QualityRiskLevel.CRITICAL.value == "critical")
    test("QualityRiskLevel is Enum", issubclass(QualityRiskLevel, Enum))

    print("\n" + "=" * 60)
    print("Test 18: Task executor integration pattern")
    print("=" * 60)

    test_chunks = [
        {"content_with_weight": "This is a normal chunk with good content and sufficient length.", "doc_id": "test1"},
        {"content_with_weight": "短", "doc_id": "test2"},
        {"content_with_weight": "Text with (cid:123) garbled pattern.", "doc_id": "test3"},
        {"content_with_weight": "ÀÁÂÃÄÅÆÇ mojibake text.", "doc_id": "test4"},
    ]

    results_int = analyzer.analyze_chunks(test_chunks)
    summary_int = analyzer.get_batch_summary(results_int)

    for idx, chunk in enumerate(test_chunks):
        if idx < len(results_int):
            r = results_int[idx]
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

    test("Integration - chunk0 has _quality_score", test_chunks[0].get("_quality_score") is not None)
    test("Integration - chunk0 score > 0.9", test_chunks[0]["_quality_score"] > 0.9)
    test("Integration - chunk0 risk_level == 'low'", test_chunks[0]["_quality_risk_level"] == "low")
    test("Integration - chunk0 issues empty", test_chunks[0]["_quality_issues"] == [])

    test("Integration - chunk2 has cid_garbled issue", "cid_garbled" in [i["type"] for i in test_chunks[2]["_quality_issues"]])
    test("Integration - chunk2 risk_level == 'critical'", test_chunks[2]["_quality_risk_level"] == "critical")

    test("Integration - chunk3 has mojibake_text or garbled_text issue", 
         any(i["type"] in ["mojibake_text", "garbled_text"] for i in test_chunks[3]["_quality_issues"]))

    test("Integration - summary total_chunks == 4", summary_int["total_chunks"] == 4)
    test("Integration - summary high_risk_count >= 1", summary_int["high_risk_count"] >= 1)

    print("\n" + "=" * 60)
    print(f"Test Results: {tests_passed} passed, {tests_failed} failed")
    print("=" * 60)

    if tests_failed > 0:
        print("\nFailed tests exist. Please review the output above.")

    return tests_failed == 0


if __name__ == "__main__":
    success = run_tests()
    sys.exit(0 if success else 1)
