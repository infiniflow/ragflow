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
Utility functions for Raptor processing decisions.
"""

import logging
from typing import Optional

# File extensions for structured data types
EXCEL_EXTENSIONS = {".xls", ".xlsx", ".xlsm", ".xlsb"}
CSV_EXTENSIONS = {".csv", ".tsv"}
STRUCTURED_EXTENSIONS = EXCEL_EXTENSIONS | CSV_EXTENSIONS


def is_structured_file_type(file_type: Optional[str]) -> bool:
    """
    Check if a file type is structured data (Excel, CSV, etc.)
    
    Args:
        file_type: File extension (e.g., ".xlsx", ".csv")
        
    Returns:
        True if file is structured data type
    """
    if not file_type:
        return False

    # Normalize to lowercase and ensure leading dot
    file_type = file_type.lower()
    if not file_type.startswith("."):
        file_type = f".{file_type}"

    return file_type in STRUCTURED_EXTENSIONS


def is_tabular_pdf(parser_id: str = "", parser_config: Optional[dict] = None) -> bool:
    """
    Check if a PDF is being parsed as tabular data.
    
    Args:
        parser_id: Parser ID (e.g., "table", "naive")
        parser_config: Parser configuration dict
        
    Returns:
        True if PDF is being parsed as tabular data
    """
    parser_config = parser_config or {}

    # If using table parser, it's tabular
    if parser_id and parser_id.lower() == "table":
        return True

    # Check if html4excel is enabled (Excel-like table parsing)
    if parser_config.get("html4excel", False):
        return True

    return False


def should_skip_raptor(
        file_type: Optional[str] = None,
        parser_id: str = "",
        parser_config: Optional[dict] = None,
        raptor_config: Optional[dict] = None
) -> bool:
    """
    Determine if Raptor should be skipped for a given document.
    
    This function implements the logic to automatically disable Raptor for:
    1. Excel files (.xls, .xlsx, .csv, etc.)
    2. PDFs with tabular data (using table parser or html4excel)
    
    Args:
        file_type: File extension (e.g., ".xlsx", ".pdf")
        parser_id: Parser ID being used
        parser_config: Parser configuration dict
        raptor_config: Raptor configuration dict (can override with auto_disable_for_structured_data)
        
    Returns:
        True if Raptor should be skipped, False otherwise
    """
    parser_config = parser_config or {}
    raptor_config = raptor_config or {}

    # Check if auto-disable is explicitly disabled in config
    if raptor_config.get("auto_disable_for_structured_data", True) is False:
        logging.info("Raptor auto-disable is turned off via configuration")
        return False

    # Check for Excel/CSV files
    if is_structured_file_type(file_type):
        logging.info(f"Skipping Raptor for structured file type: {file_type}")
        return True

    # Check for tabular PDFs
    if file_type and file_type.lower() in [".pdf", "pdf"]:
        if is_tabular_pdf(parser_id, parser_config):
            logging.info(f"Skipping Raptor for tabular PDF (parser_id={parser_id})")
            return True

    return False


def get_skip_reason(
        file_type: Optional[str] = None,
        parser_id: str = "",
        parser_config: Optional[dict] = None
) -> str:
    """
    Get a human-readable reason why Raptor was skipped.
    
    Args:
        file_type: File extension
        parser_id: Parser ID being used
        parser_config: Parser configuration dict
        
    Returns:
        Reason string, or empty string if Raptor should not be skipped
    """
    parser_config = parser_config or {}

    if is_structured_file_type(file_type):
        return f"Structured data file ({file_type}) - Raptor auto-disabled"

    if file_type and file_type.lower() in [".pdf", "pdf"]:
        if is_tabular_pdf(parser_id, parser_config):
            return f"Tabular PDF (parser={parser_id}) - Raptor auto-disabled"

    return ""
