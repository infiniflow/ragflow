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

"""
Unit tests for Raptor utility functions.
"""

import pytest
import numpy as np
from rag.utils.raptor_utils import (
    is_structured_file_type,
    is_tabular_pdf,
    should_skip_raptor,
    get_skip_reason,
    contains_html_table,
    analyze_chunks_for_tables,
    should_skip_raptor_for_chunks,
    EXCEL_EXTENSIONS,
    CSV_EXTENSIONS,
    STRUCTURED_EXTENSIONS,
    TABLE_CONTENT_THRESHOLD
)


class TestIsStructuredFileType:
    """Test file type detection for structured data"""

    @pytest.mark.parametrize("file_type,expected", [
        (".xlsx", True),
        (".xls", True),
        (".xlsm", True),
        (".xlsb", True),
        (".csv", True),
        (".tsv", True),
        ("xlsx", True),  # Without leading dot
        ("XLSX", True),  # Uppercase
        (".pdf", False),
        (".docx", False),
        (".txt", False),
        ("", False),
        (None, False),
    ])
    def test_file_type_detection(self, file_type, expected):
        """Test detection of various file types"""
        assert is_structured_file_type(file_type) == expected

    def test_excel_extensions_defined(self):
        """Test that Excel extensions are properly defined"""
        assert ".xlsx" in EXCEL_EXTENSIONS
        assert ".xls" in EXCEL_EXTENSIONS
        assert len(EXCEL_EXTENSIONS) >= 4

    def test_csv_extensions_defined(self):
        """Test that CSV extensions are properly defined"""
        assert ".csv" in CSV_EXTENSIONS
        assert ".tsv" in CSV_EXTENSIONS

    def test_structured_extensions_combined(self):
        """Test that structured extensions include both Excel and CSV"""
        assert EXCEL_EXTENSIONS.issubset(STRUCTURED_EXTENSIONS)
        assert CSV_EXTENSIONS.issubset(STRUCTURED_EXTENSIONS)


class TestIsTabularPDF:
    """Test tabular PDF detection"""

    def test_table_parser_detected(self):
        """Test that table parser is detected as tabular"""
        assert is_tabular_pdf("table", {}) is True
        assert is_tabular_pdf("TABLE", {}) is True

    def test_html4excel_detected(self):
        """Test that html4excel config is detected as tabular"""
        assert is_tabular_pdf("naive", {"html4excel": True}) is True
        assert is_tabular_pdf("", {"html4excel": True}) is True

    def test_non_tabular_pdf(self):
        """Test that non-tabular PDFs are not detected"""
        assert is_tabular_pdf("naive", {}) is False
        assert is_tabular_pdf("naive", {"html4excel": False}) is False
        assert is_tabular_pdf("", {}) is False

    def test_combined_conditions(self):
        """Test combined table parser and html4excel"""
        assert is_tabular_pdf("table", {"html4excel": True}) is True
        assert is_tabular_pdf("table", {"html4excel": False}) is True


class TestShouldSkipRaptor:
    """Test Raptor skip logic"""

    def test_skip_excel_files(self):
        """Test that Excel files skip Raptor"""
        assert should_skip_raptor(".xlsx") is True
        assert should_skip_raptor(".xls") is True
        assert should_skip_raptor(".xlsm") is True

    def test_skip_csv_files(self):
        """Test that CSV files skip Raptor"""
        assert should_skip_raptor(".csv") is True
        assert should_skip_raptor(".tsv") is True

    def test_skip_tabular_pdf_with_table_parser(self):
        """Test that tabular PDFs skip Raptor"""
        assert should_skip_raptor(".pdf", parser_id="table") is True
        assert should_skip_raptor("pdf", parser_id="TABLE") is True

    def test_skip_tabular_pdf_with_html4excel(self):
        """Test that PDFs with html4excel skip Raptor"""
        assert should_skip_raptor(".pdf", parser_config={"html4excel": True}) is True

    def test_dont_skip_regular_pdf(self):
        """Test that regular PDFs don't skip Raptor"""
        assert should_skip_raptor(".pdf", parser_id="naive") is False
        assert should_skip_raptor(".pdf", parser_config={}) is False

    def test_dont_skip_text_files(self):
        """Test that text files don't skip Raptor"""
        assert should_skip_raptor(".txt") is False
        assert should_skip_raptor(".docx") is False
        assert should_skip_raptor(".md") is False

    def test_override_with_config(self):
        """Test that auto-disable can be overridden"""
        raptor_config = {"auto_disable_for_structured_data": False}
        
        # Should not skip even for Excel files
        assert should_skip_raptor(".xlsx", raptor_config=raptor_config) is False
        assert should_skip_raptor(".csv", raptor_config=raptor_config) is False
        assert should_skip_raptor(".pdf", parser_id="table", raptor_config=raptor_config) is False

    def test_default_auto_disable_enabled(self):
        """Test that auto-disable is enabled by default"""
        # Empty raptor_config should default to auto_disable=True
        assert should_skip_raptor(".xlsx", raptor_config={}) is True
        assert should_skip_raptor(".xlsx", raptor_config=None) is True

    def test_explicit_auto_disable_enabled(self):
        """Test explicit auto-disable enabled"""
        raptor_config = {"auto_disable_for_structured_data": True}
        assert should_skip_raptor(".xlsx", raptor_config=raptor_config) is True


class TestGetSkipReason:
    """Test skip reason generation"""

    def test_excel_skip_reason(self):
        """Test skip reason for Excel files"""
        reason = get_skip_reason(".xlsx")
        assert "Structured data file" in reason
        assert ".xlsx" in reason
        assert "auto-disabled" in reason.lower()

    def test_csv_skip_reason(self):
        """Test skip reason for CSV files"""
        reason = get_skip_reason(".csv")
        assert "Structured data file" in reason
        assert ".csv" in reason

    def test_tabular_pdf_skip_reason(self):
        """Test skip reason for tabular PDFs"""
        reason = get_skip_reason(".pdf", parser_id="table")
        assert "Tabular PDF" in reason
        assert "table" in reason.lower()
        assert "auto-disabled" in reason.lower()

    def test_html4excel_skip_reason(self):
        """Test skip reason for html4excel PDFs"""
        reason = get_skip_reason(".pdf", parser_config={"html4excel": True})
        assert "Tabular PDF" in reason

    def test_no_skip_reason_for_regular_files(self):
        """Test that regular files have no skip reason"""
        assert get_skip_reason(".txt") == ""
        assert get_skip_reason(".docx") == ""
        assert get_skip_reason(".pdf", parser_id="naive") == ""


class TestEdgeCases:
    """Test edge cases and error handling"""

    def test_none_values(self):
        """Test handling of None values"""
        assert should_skip_raptor(None) is False
        assert should_skip_raptor("") is False
        assert get_skip_reason(None) == ""

    def test_empty_strings(self):
        """Test handling of empty strings"""
        assert should_skip_raptor("") is False
        assert get_skip_reason("") == ""

    def test_case_insensitivity(self):
        """Test case insensitive handling"""
        assert is_structured_file_type("XLSX") is True
        assert is_structured_file_type("XlSx") is True
        assert is_tabular_pdf("TABLE", {}) is True
        assert is_tabular_pdf("TaBlE", {}) is True

    def test_with_and_without_dot(self):
        """Test file extensions with and without leading dot"""
        assert should_skip_raptor(".xlsx") is True
        assert should_skip_raptor("xlsx") is True
        assert should_skip_raptor(".CSV") is True
        assert should_skip_raptor("csv") is True


class TestIntegrationScenarios:
    """Test real-world integration scenarios"""

    def test_financial_excel_report(self):
        """Test scenario: Financial quarterly Excel report"""
        file_type = ".xlsx"
        parser_id = "naive"
        parser_config = {}
        raptor_config = {"use_raptor": True}
        
        # Should skip Raptor
        assert should_skip_raptor(file_type, parser_id, parser_config, raptor_config) is True
        reason = get_skip_reason(file_type, parser_id, parser_config)
        assert "Structured data file" in reason

    def test_scientific_csv_data(self):
        """Test scenario: Scientific experimental CSV results"""
        file_type = ".csv"
        
        # Should skip Raptor
        assert should_skip_raptor(file_type) is True
        reason = get_skip_reason(file_type)
        assert ".csv" in reason

    def test_legal_contract_with_tables(self):
        """Test scenario: Legal contract PDF with tables"""
        file_type = ".pdf"
        parser_id = "table"
        parser_config = {}
        
        # Should skip Raptor
        assert should_skip_raptor(file_type, parser_id, parser_config) is True
        reason = get_skip_reason(file_type, parser_id, parser_config)
        assert "Tabular PDF" in reason

    def test_text_heavy_pdf_document(self):
        """Test scenario: Text-heavy PDF document"""
        file_type = ".pdf"
        parser_id = "naive"
        parser_config = {}
        
        # Should NOT skip Raptor
        assert should_skip_raptor(file_type, parser_id, parser_config) is False
        reason = get_skip_reason(file_type, parser_id, parser_config)
        assert reason == ""

    def test_mixed_dataset_processing(self):
        """Test scenario: Mixed dataset with various file types"""
        files = [
            (".xlsx", "naive", {}, True),  # Excel - skip
            (".csv", "naive", {}, True),   # CSV - skip
            (".pdf", "table", {}, True),   # Tabular PDF - skip
            (".pdf", "naive", {}, False),  # Regular PDF - don't skip
            (".docx", "naive", {}, False), # Word doc - don't skip
            (".txt", "naive", {}, False),  # Text file - don't skip
        ]
        
        for file_type, parser_id, parser_config, expected_skip in files:
            result = should_skip_raptor(file_type, parser_id, parser_config)
            assert result == expected_skip, f"Failed for {file_type}"

    def test_override_for_special_excel(self):
        """Test scenario: Override auto-disable for special Excel processing"""
        file_type = ".xlsx"
        raptor_config = {"auto_disable_for_structured_data": False}
        
        # Should NOT skip when explicitly disabled
        assert should_skip_raptor(file_type, raptor_config=raptor_config) is False


class TestContainsHtmlTable:
    """Test HTML table detection in content"""

    def test_detect_simple_table(self):
        """Test detection of simple HTML table"""
        content = "<table><tr><td>Cell 1</td><td>Cell 2</td></tr></table>"
        assert contains_html_table(content) is True

    def test_detect_table_with_attributes(self):
        """Test detection of table with attributes"""
        content = '<table class="data-table" border="1"><tr><td>Data</td></tr></table>'
        assert contains_html_table(content) is True

    def test_detect_table_case_insensitive(self):
        """Test case insensitive detection"""
        assert contains_html_table("<TABLE><TR><TD>X</TD></TR></TABLE>") is True
        assert contains_html_table("<Table><tr><td>X</td></tr></Table>") is True

    def test_no_table_in_plain_text(self):
        """Test that plain text is not detected as table"""
        content = "This is just plain text without any tables."
        assert contains_html_table(content) is False

    def test_no_table_in_empty_content(self):
        """Test empty content handling"""
        assert contains_html_table("") is False
        # Note: None is rejected by type hints (beartype), which is correct behavior

    def test_table_word_not_detected(self):
        """Test that the word 'table' alone is not detected"""
        content = "Please see the table below for more information."
        assert contains_html_table(content) is False

    def test_mixed_content_with_table(self):
        """Test content with text and table"""
        content = """
        This is some introductory text.
        <table>
            <caption>Financial Data</caption>
            <tr><th>Year</th><th>Revenue</th></tr>
            <tr><td>2024</td><td>$1M</td></tr>
        </table>
        More text after the table.
        """
        assert contains_html_table(content) is True


class TestAnalyzeChunksForTables:
    """Test chunk analysis for table content"""

    def _make_chunk(self, content: str):
        """Helper to create a chunk tuple"""
        return (content, np.zeros(768))

    def test_all_table_chunks(self):
        """Test when all chunks contain tables"""
        chunks = [
            self._make_chunk("<table><tr><td>1</td></tr></table>"),
            self._make_chunk("<table><tr><td>2</td></tr></table>"),
            self._make_chunk("<table><tr><td>3</td></tr></table>"),
        ]
        should_skip, pct = analyze_chunks_for_tables(chunks)
        assert should_skip is True
        assert pct == 1.0

    def test_no_table_chunks(self):
        """Test when no chunks contain tables"""
        chunks = [
            self._make_chunk("Plain text content 1"),
            self._make_chunk("Plain text content 2"),
            self._make_chunk("Plain text content 3"),
        ]
        should_skip, pct = analyze_chunks_for_tables(chunks)
        assert should_skip is False
        assert pct == 0.0

    def test_mixed_chunks_below_threshold(self):
        """Test mixed chunks below threshold"""
        # 1 out of 5 = 20%, below 30% threshold
        chunks = [
            self._make_chunk("<table><tr><td>Table</td></tr></table>"),
            self._make_chunk("Plain text 1"),
            self._make_chunk("Plain text 2"),
            self._make_chunk("Plain text 3"),
            self._make_chunk("Plain text 4"),
        ]
        should_skip, pct = analyze_chunks_for_tables(chunks)
        assert should_skip is False
        assert pct == 0.2

    def test_mixed_chunks_above_threshold(self):
        """Test mixed chunks above threshold"""
        # 2 out of 5 = 40%, above 30% threshold
        chunks = [
            self._make_chunk("<table><tr><td>Table 1</td></tr></table>"),
            self._make_chunk("<table><tr><td>Table 2</td></tr></table>"),
            self._make_chunk("Plain text 1"),
            self._make_chunk("Plain text 2"),
            self._make_chunk("Plain text 3"),
        ]
        should_skip, pct = analyze_chunks_for_tables(chunks)
        assert should_skip is True
        assert pct == 0.4

    def test_empty_chunks(self):
        """Test empty chunk list"""
        should_skip, pct = analyze_chunks_for_tables([])
        assert should_skip is False
        assert pct == 0.0

    def test_custom_threshold(self):
        """Test with custom threshold"""
        # 1 out of 5 = 20%
        chunks = [
            self._make_chunk("<table><tr><td>Table</td></tr></table>"),
            self._make_chunk("Plain text 1"),
            self._make_chunk("Plain text 2"),
            self._make_chunk("Plain text 3"),
            self._make_chunk("Plain text 4"),
        ]
        # With 15% threshold, should skip
        should_skip, pct = analyze_chunks_for_tables(chunks, threshold=0.15)
        assert should_skip is True
        
        # With 25% threshold, should not skip
        should_skip, pct = analyze_chunks_for_tables(chunks, threshold=0.25)
        assert should_skip is False

    def test_default_threshold_value(self):
        """Test that default threshold is 30%"""
        assert TABLE_CONTENT_THRESHOLD == 0.3


class TestShouldSkipRaptorForChunks:
    """Test content-based Raptor skip decision"""

    def _make_chunk(self, content: str):
        """Helper to create a chunk tuple"""
        return (content, np.zeros(768))

    def test_skip_for_table_heavy_content(self):
        """Test skipping for table-heavy content"""
        chunks = [
            self._make_chunk("<table><tr><td>1</td></tr></table>"),
            self._make_chunk("<table><tr><td>2</td></tr></table>"),
            self._make_chunk("Plain text"),
        ]
        should_skip, reason = should_skip_raptor_for_chunks(chunks)
        assert should_skip is True
        assert "HTML tables" in reason

    def test_no_skip_for_text_content(self):
        """Test not skipping for text content"""
        chunks = [
            self._make_chunk("Plain text content 1"),
            self._make_chunk("Plain text content 2"),
            self._make_chunk("Plain text content 3"),
        ]
        should_skip, reason = should_skip_raptor_for_chunks(chunks)
        assert should_skip is False
        assert reason == ""

    def test_override_with_config(self):
        """Test that auto-disable can be overridden"""
        chunks = [
            self._make_chunk("<table><tr><td>1</td></tr></table>"),
            self._make_chunk("<table><tr><td>2</td></tr></table>"),
        ]
        raptor_config = {"auto_disable_for_structured_data": False}
        should_skip, reason = should_skip_raptor_for_chunks(chunks, raptor_config)
        assert should_skip is False
        assert reason == ""

    def test_empty_chunks(self):
        """Test with empty chunks"""
        should_skip, reason = should_skip_raptor_for_chunks([])
        assert should_skip is False
        assert reason == ""


class TestPDFWithHtmlTables:
    """Test real-world PDF with HTML tables scenario (ahmadshakil's issue)"""

    def _make_chunk(self, content: str):
        """Helper to create a chunk tuple"""
        return (content, np.zeros(768))

    def test_pdf_with_extracted_tables(self):
        """Test PDF that has tables extracted as HTML during parsing"""
        # Simulating chunks from a PDF like Fbr_IncomeTaxOrdinance_2001
        chunks = [
            self._make_chunk("Section 1: Introduction to Tax Law"),
            self._make_chunk('<table><caption>Table Location: Section 2</caption><tr><th>Tax Rate</th><th>Income Range</th></tr><tr><td>10%</td><td>0-500,000</td></tr></table>'),
            self._make_chunk("Section 3: Deductions and Exemptions"),
            self._make_chunk('<table><tr><th>Deduction Type</th><th>Maximum Amount</th></tr><tr><td>Medical</td><td>100,000</td></tr></table>'),
            self._make_chunk("Section 4: Filing Requirements"),
        ]
        
        # 2 out of 5 = 40%, above 30% threshold
        should_skip, reason = should_skip_raptor_for_chunks(chunks)
        assert should_skip is True
        assert "HTML tables" in reason

    def test_pdf_with_few_tables(self):
        """Test PDF with only occasional tables"""
        chunks = [
            self._make_chunk("Chapter 1: Overview of the legal framework..."),
            self._make_chunk("Chapter 2: Detailed analysis of provisions..."),
            self._make_chunk("Chapter 3: Case studies and examples..."),
            self._make_chunk("Chapter 4: Implementation guidelines..."),
            self._make_chunk("Chapter 5: Compliance requirements..."),
            self._make_chunk("Chapter 6: Penalties and enforcement..."),
            self._make_chunk("Chapter 7: Appeals process..."),
            self._make_chunk("Chapter 8: Recent amendments..."),
            self._make_chunk("Chapter 9: Future outlook..."),
            self._make_chunk('<table><tr><td>Summary Table</td></tr></table>'),  # Only 1 table
        ]
        
        # 1 out of 10 = 10%, below 30% threshold
        should_skip, reason = should_skip_raptor_for_chunks(chunks)
        assert should_skip is False

    def test_financial_pdf_with_many_tables(self):
        """Test financial PDF with many tables (should skip)"""
        chunks = [
            self._make_chunk('<table><caption>Balance Sheet</caption><tr><td>Assets</td><td>$1M</td></tr></table>'),
            self._make_chunk('<table><caption>Income Statement</caption><tr><td>Revenue</td><td>$500K</td></tr></table>'),
            self._make_chunk('<table><caption>Cash Flow</caption><tr><td>Operating</td><td>$200K</td></tr></table>'),
            self._make_chunk("Notes to financial statements..."),
            self._make_chunk('<table><caption>Tax Schedule</caption><tr><td>Tax</td><td>$50K</td></tr></table>'),
        ]
        
        # 4 out of 5 = 80%, well above threshold
        should_skip, reason = should_skip_raptor_for_chunks(chunks)
        assert should_skip is True


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
