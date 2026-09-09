"""
RAGFlow ES to OceanBase Migration Tool

A CLI tool for migrating RAGFlow data from Elasticsearch 8+ to OceanBase,
supporting schema conversion, vector data mapping, batch import, and resume capability.

This tool is specifically designed for RAGFlow's data structure.
"""

__version__ = "0.1.0"

from .migrator import ESToOceanBaseMigrator
from .es_client import ESClient
from .ob_client import OBClient
from .schema import RAGFlowSchemaConverter, RAGFlowDataConverter, RAGFLOW_COLUMNS
from .verify import MigrationVerifier, VerificationResult
from .progress import ProgressManager, MigrationProgress

# Backwards compatibility aliases
SchemaConverter = RAGFlowSchemaConverter
DataConverter = RAGFlowDataConverter

__all__ = [
    # Main classes
    "ESToOceanBaseMigrator",
    "ESClient",
    "OBClient",
    # Schema
    "RAGFlowSchemaConverter",
    "RAGFlowDataConverter",
    "RAGFLOW_COLUMNS",
    # Verification
    "MigrationVerifier",
    "VerificationResult",
    # Progress
    "ProgressManager",
    "MigrationProgress",
    # Aliases
    "SchemaConverter",
    "DataConverter",
]
