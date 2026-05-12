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

import json
import logging
from typing import Optional

import xxhash

RAPTOR_TREE_BUILDER = "raptor"
PSI_TREE_BUILDER = "psi"
SUPPORTED_TREE_BUILDERS = {RAPTOR_TREE_BUILDER, PSI_TREE_BUILDER}
GMM_CLUSTERING_METHOD = "gmm"
AHC_CLUSTERING_METHOD = "ahc"
SUPPORTED_CLUSTERING_METHODS = {GMM_CLUSTERING_METHOD, AHC_CLUSTERING_METHOD}

# File extensions for structured data types
EXCEL_EXTENSIONS = {".xls", ".xlsx", ".xlsm", ".xlsb"}
CSV_EXTENSIONS = {".csv", ".tsv"}
STRUCTURED_EXTENSIONS = EXCEL_EXTENSIONS | CSV_EXTENSIONS


def get_raptor_tree_builder(raptor_config: dict | None) -> str:
    """Return the configured RAPTOR tree builder with legacy ext fallback."""
    raptor_config = raptor_config or {}
    ext = raptor_config.get("ext") or {}
    tree_builder = ext.get("tree_builder") or raptor_config.get("tree_builder") or RAPTOR_TREE_BUILDER
    if tree_builder not in SUPPORTED_TREE_BUILDERS:
        raise ValueError(f"Unsupported RAPTOR tree builder: {tree_builder}")
    return tree_builder


def get_raptor_clustering_method(raptor_config: dict | None) -> str:
    """Return the configured RAPTOR clustering method with legacy ext fallback."""
    raptor_config = raptor_config or {}
    ext = raptor_config.get("ext") or {}
    clustering_method = ext.get("clustering_method") or raptor_config.get("clustering_method") or GMM_CLUSTERING_METHOD
    if clustering_method not in SUPPORTED_CLUSTERING_METHODS:
        raise ValueError(f"Unsupported RAPTOR clustering method: {clustering_method}")
    return clustering_method


def _as_extra_dict(extra) -> dict:
    """Normalize a chunk extra payload into a dictionary."""
    if isinstance(extra, dict):
        return extra
    if isinstance(extra, str) and extra:
        try:
            parsed = json.loads(extra)
        except json.JSONDecodeError:
            logging.warning(
                "Ignoring malformed RAPTOR extra payload while collecting chunk metadata: %s",
                extra[:200],
                exc_info=True,
            )
            return {}
        return parsed if isinstance(parsed, dict) else {}
    return {}


def _has_raptor_marker(marker) -> bool:
    """Return whether a chunk marker identifies a RAPTOR summary chunk."""
    if isinstance(marker, list):
        return any(str(item) == RAPTOR_TREE_BUILDER for item in marker)
    return str(marker) == RAPTOR_TREE_BUILDER


def _raptor_methods_from_fields(fields: dict, extra: dict | None = None) -> set[str]:
    """Read RAPTOR builder methods from stored chunk fields."""
    extra = extra if extra is not None else _as_extra_dict(fields.get("extra"))
    method = extra.get("raptor_method") or RAPTOR_TREE_BUILDER
    if isinstance(method, list):
        return {str(item) for item in method if item}
    return {str(method)} if method else set()


def collect_raptor_methods(field_map: dict) -> set[str]:
    """Collect tree-builder methods from RAPTOR summary chunk fields."""
    methods = set()
    for fields in field_map.values():
        extra = _as_extra_dict(fields.get("extra"))
        marker = fields.get("raptor_kwd") or extra.get("raptor_kwd")
        if not _has_raptor_marker(marker):
            continue

        methods.update(_raptor_methods_from_fields(fields, extra))
    return methods


def collect_raptor_chunk_ids(field_map: dict, exclude_methods: set[str] | None = None) -> set[str]:
    """Collect RAPTOR summary chunk IDs, optionally excluding some methods."""
    chunk_ids = set()
    exclude_methods = exclude_methods or set()
    for chunk_id, fields in field_map.items():
        extra = _as_extra_dict(fields.get("extra"))
        marker = fields.get("raptor_kwd") or extra.get("raptor_kwd")
        if _has_raptor_marker(marker):
            if _raptor_methods_from_fields(fields, extra).issubset(exclude_methods):
                continue
            chunk_ids.add(chunk_id)
    return chunk_ids


def make_raptor_summary_chunk_id(content: str, doc_id: str) -> str:
    """Build the stable ID used for generated RAPTOR summary chunks."""
    return xxhash.xxh64((content + str(doc_id)).encode("utf-8")).hexdigest()


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
