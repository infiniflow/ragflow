#
# Compatibility shim for legacy imports from api.db.db_models
#
# This file re-exports all symbols from the new modular structure in api/db/.
# New code should import from specific modules or from api.db.models.
#

# Core database connection and utilities
from api.db.connection import (
    DB,
    BaseDataBase,
    close_connection,
)
from api.db.pool import (
    with_retry,
    RetryingPooledMySQLDatabase,
    RetryingPooledPostgresqlDatabase,
    PooledDatabase,
    DatabaseMigrator,
)
from api.db.locks import (
    DatabaseLock,
    MysqlDatabaseLock,
    PostgresDatabaseLock,
)

# Field types and helper functions
from api.db.fields import (
    JSONField,
    ListField,
    SerializedField,
    LongTextField,
    JsonSerializedField,
    is_continuous_field,
    auto_date_timestamp_field,
    auto_date_timestamp_db_field,
    remove_field_name_prefix,
    DateTimeTzField,
    AUTO_DATE_TIMESTAMP_FIELD_PREFIX,
    CONTINUOUS_FIELD_TYPE,
    coerce_timestamp_range,
)

# Base models
from api.db.base import (
    BaseModel,
    DataBaseModel,
)

# Domain models
from api.db.models import (
    User,
    Tenant,
    UserTenant,
    InvitationCode,
    LLMFactories,
    LLM,
    TenantLLM,
    TenantLangfuse,
    Knowledgebase,
    Document,
    File,
    File2Document,
    Task,
    Dialog,
    Conversation,
    APIToken,
    API4Conversation,
    UserCanvas,
    CanvasTemplate,
    UserCanvasVersion,
    MCPServer,
    Search,
    PipelineOperationLog,
    Connector,
    Connector2Kb,
    SyncLogs,
    EvaluationDataset,
    EvaluationCase,
    EvaluationRun,
    EvaluationResult,
    Memory,
    SystemSettings,
)

# Migration utilities
from api.db.migrations import (
    init_database_tables,
    fill_db_model_object,
    migrate_db,
    alter_db_add_column,
    alter_db_column_type,
    alter_db_rename_column,
)

# Explicitly export everything for 'from api.db.db_models import *'
__all__ = [
    "DB",
    "BaseDataBase",
    "close_connection",
    "with_retry",
    "RetryingPooledMySQLDatabase",
    "RetryingPooledPostgresqlDatabase",
    "PooledDatabase",
    "DatabaseMigrator",
    "DatabaseLock",
    "MysqlDatabaseLock",
    "PostgresDatabaseLock",
    "JSONField",
    "ListField",
    "SerializedField",
    "LongTextField",
    "JsonSerializedField",
    "is_continuous_field",
    "auto_date_timestamp_field",
    "auto_date_timestamp_db_field",
    "AUTO_DATE_TIMESTAMP_FIELD_PREFIX",
    "CONTINUOUS_FIELD_TYPE",
    "coerce_timestamp_range",
    "remove_field_name_prefix",
    "DateTimeTzField",
    "BaseModel",
    "DataBaseModel",
    "User",
    "Tenant",
    "UserTenant",
    "InvitationCode",
    "LLMFactories",
    "LLM",
    "TenantLLM",
    "TenantLangfuse",
    "Knowledgebase",
    "Document",
    "File",
    "File2Document",
    "Task",
    "Dialog",
    "Conversation",
    "APIToken",
    "API4Conversation",
    "UserCanvas",
    "CanvasTemplate",
    "UserCanvasVersion",
    "MCPServer",
    "Search",
    "PipelineOperationLog",
    "Connector",
    "Connector2Kb",
    "SyncLogs",
    "EvaluationDataset",
    "EvaluationCase",
    "EvaluationRun",
    "EvaluationResult",
    "Memory",
    "SystemSettings",
    "init_database_tables",
    "fill_db_model_object",
    "migrate_db",
    "alter_db_add_column",
    "alter_db_column_type",
    "alter_db_rename_column",
]
