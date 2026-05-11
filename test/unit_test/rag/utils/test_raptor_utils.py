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
from rag.utils.raptor_utils import (
    is_structured_file_type,
    is_tabular_pdf,
    should_skip_raptor,
    get_skip_reason,
    EXCEL_EXTENSIONS,
    CSV_EXTENSIONS,
    STRUCTURED_EXTENSIONS
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


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
