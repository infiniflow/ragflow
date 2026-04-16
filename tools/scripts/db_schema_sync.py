#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
Database Schema Sync Script

This script synchronizes database models defined in api/db/db_models.py
with the actual database schema using peewee-migrate.

Features:
1. Reads model definitions from api/db/db_models.py
2. Compares with existing database tables specified via command line
3. Generates migration files in tools/migrate/{version}/
"""

import argparse
import importlib.util
import inspect
import logging
import os
import re
import sys

from peewee import MySQLDatabase, Model, Field
from peewee_migrate import Router

# Add project root to path for imports
PROJECT_BASE = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.path.insert(0, PROJECT_BASE)

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def validate_version(version: str) -> bool:
    """Validate version format: vxx.xx.xx where xx are digits"""
    pattern = r'^v\d+\.\d+\.\d+$'
    return bool(re.match(pattern, version))


def version_to_dirname(version: str) -> str:
    """Convert version string to valid directory name (e.g., 'v0.24.0' -> 'v0_24_0')"""
    return version.replace('.', '_')


def load_db_models():
    """Load database models from api/db/db_models.py"""
    models_path = os.path.join(PROJECT_BASE, 'api', 'db', 'db_models.py')
    
    if not os.path.exists(models_path):
        raise FileNotFoundError(f"db_models.py not found at {models_path}")
    
    # Import the module
    spec = importlib.util.spec_from_file_location("db_models", models_path)
    db_models = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(db_models)
    
    # Get all Model subclasses
    models = []
    for name, obj in inspect.getmembers(db_models):
        if inspect.isclass(obj) and issubclass(obj, Model) and obj is not Model:
            # Skip base model classes
            if obj.__name__ in ['BaseModel', 'DataBaseModel']:
                continue
            # Check if it has a database attribute (is a proper model)
            if hasattr(obj._meta, 'database'):
                models.append(obj)
    
    return models, db_models


def create_database_connection(host: str, port: int, user: str, password: str, database: str):
    """Create MySQL database connection from command line arguments"""
    db = MySQLDatabase(
        database,
        host=host,
        port=port,
        user=user,
        password=password,
        charset='utf8mb4'
    )
    return db


# MySQL type to Peewee field type mapping
MYSQL_TO_PEEWEE_TYPE = {
    'varchar': 'CharField',
    'char': 'CharField',
    'text': 'TextField',
    'longtext': 'TextField',
    'mediumtext': 'TextField',
    'int': 'IntegerField',
    'integer': 'IntegerField',
    'bigint': 'BigIntegerField',
    'float': 'FloatField',
    'double': 'FloatField',
    'decimal': 'FloatField',
    'datetime': 'DateTimeField',
    'timestamp': 'DateTimeField',
    'tinyint(1)': 'BooleanField',
    'tinyint': 'IntegerField',
    'smallint': 'IntegerField',
    'mediumint': 'IntegerField',
}

PEEWEE_TO_MYSQL_TYPE = {
    'CharField': 'varchar',
    'TextField': 'text',
    'IntegerField': 'int',
    'BigIntegerField': 'bigint',
    'FloatField': 'float',
    'BooleanField': 'tinyint',
    'DateTimeField': 'datetime',
}


def get_table_columns(db, table_name: str) -> dict:
    """Get column information from database table
    
    Returns:
        dict: {column_name: {type, nullable, default, ...}}
    """
    cursor = db.execute_sql("""
        SELECT 
            column_name,
            data_type,
            column_type,
            is_nullable,
            column_default,
            column_key,
            extra
        FROM information_schema.columns
        WHERE table_schema = %s AND table_name = %s
        ORDER BY ordinal_position
    """, (db.database, table_name))
    
    columns = {}
    for row in cursor.fetchall():
        col_name = row[0]
        data_type = row[1].lower()
        column_type = row[2].lower()
        is_nullable = row[3] == 'YES'
        column_default = row[4]
        column_key = row[5]
        extra = row[6] or ''
        
        # Determine peewee type
        if column_type.startswith('tinyint(1)'):
            peewee_type = 'BooleanField'
        else:
            peewee_type = MYSQL_TO_PEEWEE_TYPE.get(data_type, 'TextField')
        
        columns[col_name] = {
            'data_type': data_type,
            'column_type': column_type,
            'peewee_type': peewee_type,
            'nullable': is_nullable,
            'default': column_default,
            'is_primary': column_key == 'PRI',
            'extra': extra,
        }
    
    return columns


def get_peewee_field_type(field: Field) -> str:
    """Get peewee field type name"""
    field_class = field.__class__.__name__
    return field_class


def get_base_field_type(field: Field) -> str:
    """Get base peewee field type by walking the MRO chain.
    
    Custom field types (like DateTimeTzField, JSONField) inherit from standard types.
    This function returns the underlying standard type for comparison.
    """
    # Standard peewee field types we consider as "base" types
    STANDARD_TYPES = {
        'CharField', 'TextField', 'IntegerField', 'BigIntegerField',
        'FloatField', 'BooleanField', 'DateTimeField', 'DateField',
        'TimeField', 'DecimalField', 'ForeignKeyField', 'ManyToManyField',
        'PrimaryKeyField', 'AutoField'
    }
    
    # Walk through the MRO (Method Resolution Order) to find standard type
    for cls in field.__class__.__mro__:
        class_name = cls.__name__
        if class_name in STANDARD_TYPES:
            return class_name
    
    # Fallback to TextField if no standard type found
    return 'TextField'


def normalize_field_type(field: Field) -> str:
    """Normalize field type for comparison using base type"""
    return get_base_field_type(field)


def compare_fields(model_fields: dict, db_columns: dict) -> dict:
    """Compare model fields with database columns
    
    Returns:
        dict: {
            'added': {field_name: field_obj},  # New fields not in DB
            'changed': {field_name: (old_info, new_field)},  # Type changed
        }
    """
    result = {
        'added': {},
        'changed': {},
    }
    
    # Skip auto-generated fields like id, create_time, etc.
    skip_fields = {'id'}
    
    for field_name, field in model_fields.items():
        if field_name in skip_fields:
            continue
        
        # Check if field exists in database
        if field_name not in db_columns:
            result['added'][field_name] = field
            logger.info(f"  New field detected: {field_name} ({field.__class__.__name__})")
        else:
            # Check if type changed
            db_col = db_columns[field_name]
            model_base_type = normalize_field_type(field)
            db_type = db_col['peewee_type']
            
            # Type mismatch
            if model_base_type != db_type:
                result['changed'][field_name] = (db_col, field)
                logger.info(f"  Field type changed: {field_name} ({db_type} -> {model_base_type}, actual: {field.__class__.__name__})")
    
    return result


def generate_field_code(field: Field, field_name: str) -> str:
    """Generate peewee field definition code"""
    field_class = field.__class__.__name__
    
    # Map custom field types to standard peewee types for migration
    # These custom types will be stored as their underlying standard type
    custom_to_standard = {
        'LongTextField': 'TextField',
        'JSONField': 'TextField',
        'ListField': 'TextField',
        'SerializedField': 'TextField',
        'DateTimeTzField': 'CharField',
    }
    
    # Use standard type for custom fields
    pw_field_class = custom_to_standard.get(field_class, field_class)
    
    # Build field arguments
    args = []
    
    # max_length for CharField
    if pw_field_class == 'CharField' and hasattr(field, 'max_length') and field.max_length is not None:
        args.append(f"max_length={field.max_length}")
    
    # null
    if field.null:
        args.append("null=True")
    
    # default
    if field.default is not None:
        default_val = field.default
        if isinstance(default_val, str):
            # Escape quotes in string
            escaped = default_val.replace("'", "\\'")
            args.append(f"default='{escaped}'")
        elif isinstance(default_val, bool):
            args.append(f"default={'True' if default_val else 'False'}")
        elif isinstance(default_val, (int, float)):
            args.append(f"default={default_val}")
        elif isinstance(default_val, dict):
            args.append(f"default={default_val}")
        elif isinstance(default_val, list):
            args.append(f"default={default_val}")
    
    # index
    if getattr(field, 'index', False):
        args.append("index=True")
    
    # unique
    if getattr(field, 'unique', False):
        args.append("unique=True")
    
    args_str = ', '.join(args)
    return f"pw.{pw_field_class}({args_str})"


def generate_add_field_sql(table_name: str, field: Field, field_name: str) -> str:
    """Generate raw SQL for adding a field to MySQL table.
    
    This is used for existing tables where migrator.add_fields doesn't work
    because the model is not registered in migrator.orm.
    """
    field_class = field.__class__.__name__
    
    # Determine MySQL column type
    mysql_type_map = {
        'CharField': f'VARCHAR({field.max_length})' if hasattr(field, 'max_length') and field.max_length else 'VARCHAR(255)',
        'TextField': 'LONGTEXT',
        'LongTextField': 'LONGTEXT',
        'JSONField': 'LONGTEXT',
        'ListField': 'LONGTEXT',
        'SerializedField': 'LONGTEXT',
        'IntegerField': 'INT',
        'BigIntegerField': 'BIGINT',
        'FloatField': 'DOUBLE',
        'BooleanField': 'TINYINT(1)',
        'DateTimeField': 'DATETIME',
        'DateTimeTzField': f'VARCHAR({field.max_length})' if hasattr(field, 'max_length') and field.max_length else 'VARCHAR(255)',
    }
    
    mysql_type = mysql_type_map.get(field_class, 'LONGTEXT')
    
    # Build column definition
    parts = [f'`{field_name}`', mysql_type]
    
    # NULL/NOT NULL
    if field.null:
        parts.append('NULL')
    else:
        parts.append('NOT NULL')
    
    # DEFAULT
    if field.default is not None:
        default_val = field.default
        if isinstance(default_val, str):
            escaped = default_val.replace("'", "''")
            parts.append(f"DEFAULT '{escaped}'")
        elif isinstance(default_val, bool):
            parts.append(f"DEFAULT {1 if default_val else 0}")
        elif isinstance(default_val, (int, float)):
            parts.append(f"DEFAULT {default_val}")
        elif isinstance(default_val, dict) or isinstance(default_val, list):
            import json
            escaped = json.dumps(default_val).replace("'", "''")
            parts.append(f"DEFAULT '{escaped}'")
    
    # COMMENT
    if hasattr(field, 'help_text') and field.help_text:
        escaped = field.help_text.replace("'", "''")
        parts.append(f"COMMENT '{escaped}'")
    
    sql = f"ALTER TABLE `{table_name}` ADD COLUMN {' '.join(parts)}"
    
    # Add index if needed
    index_sql = None
    if getattr(field, 'index', False):
        index_sql = f"CREATE INDEX `idx_{table_name}_{field_name}` ON `{table_name}` (`{field_name}`)"
    
    return sql, index_sql


def generate_rollback_field_sql(table_name: str, field_name: str) -> str:
    """Generate SQL for removing a field."""
    return f"ALTER TABLE `{table_name}` DROP COLUMN `{field_name}`"


def generate_rollback_modify_sql(table_name: str, old_info: dict, field_name: str) -> str:
    """Generate SQL for rolling back a field type change.
    
    Note: This restores the column type, but data values may need manual handling
    if the type conversion caused data loss or transformation.
    """
    # Reconstruct MySQL type from old_info
    mysql_type = old_info.get('column_type', 'LONGTEXT')
    
    # Build column definition
    parts = [f'`{field_name}`', mysql_type]
    
    # NULL/NOT NULL
    if old_info.get('nullable', True):
        parts.append('NULL')
    else:
        parts.append('NOT NULL')
    
    # DEFAULT (if available)
    if old_info.get('default') is not None:
        default_val = old_info['default']
        if isinstance(default_val, str):
            escaped = default_val.replace("'", "''")
            parts.append(f"DEFAULT '{escaped}'")
        elif isinstance(default_val, bool):
            parts.append(f"DEFAULT {1 if default_val else 0}")
        elif isinstance(default_val, (int, float)):
            parts.append(f"DEFAULT {default_val}")
    
    return f"ALTER TABLE `{table_name}` MODIFY COLUMN {' '.join(parts)}"


def generate_modify_field_sql(table_name: str, field: Field, field_name: str) -> str:
    """Generate SQL for modifying a field in MySQL table."""
    field_class = field.__class__.__name__
    
    # Determine MySQL column type
    mysql_type_map = {
        'CharField': f'VARCHAR({field.max_length})' if hasattr(field, 'max_length') and field.max_length else 'VARCHAR(255)',
        'TextField': 'LONGTEXT',
        'LongTextField': 'LONGTEXT',
        'JSONField': 'LONGTEXT',
        'ListField': 'LONGTEXT',
        'SerializedField': 'LONGTEXT',
        'IntegerField': 'INT',
        'BigIntegerField': 'BIGINT',
        'FloatField': 'DOUBLE',
        'BooleanField': 'TINYINT(1)',
        'DateTimeField': 'DATETIME',
        'DateTimeTzField': f'VARCHAR({field.max_length})' if hasattr(field, 'max_length') and field.max_length else 'VARCHAR(255)',
    }
    
    mysql_type = mysql_type_map.get(field_class, 'LONGTEXT')
    
    # Build column definition
    parts = [f'`{field_name}`', mysql_type]
    
    # NULL/NOT NULL
    if field.null:
        parts.append('NULL')
    else:
        parts.append('NOT NULL')
    
    # DEFAULT
    if field.default is not None:
        default_val = field.default
        if isinstance(default_val, str):
            escaped = default_val.replace("'", "''")
            parts.append(f"DEFAULT '{escaped}'")
        elif isinstance(default_val, bool):
            parts.append(f"DEFAULT {1 if default_val else 0}")
        elif isinstance(default_val, (int, float)):
            parts.append(f"DEFAULT {default_val}")
        elif isinstance(default_val, dict) or isinstance(default_val, list):
            import json
            escaped = json.dumps(default_val).replace("'", "''")
            parts.append(f"DEFAULT '{escaped}'")
    
    # COMMENT
    if hasattr(field, 'help_text') and field.help_text:
        escaped = field.help_text.replace("'", "''")
        parts.append(f"COMMENT '{escaped}'")
    
    return f"ALTER TABLE `{table_name}` MODIFY COLUMN {' '.join(parts)}"


def generate_migration_content(new_tables: list, field_changes: dict, migrate_dir: str, migration_name: str) -> str:
    """Generate migration file content"""
    lines = [
        '"""Peewee migrations."""',
        '',
        'from contextlib import suppress',
        '',
        'import peewee as pw',
        'from peewee_migrate import Migrator',
        '',
        '',
        'with suppress(ImportError):',
        '    import playhouse.postgres_ext as pw_pext',
        '',
        '',
        'def migrate(migrator: Migrator, database: pw.Database, *, fake=False):',
        '    """Write your migrations here."""',
        '',
    ]
    
    # Generate create_model for new tables
    for model in new_tables:
        table_name = model._meta.table_name
        model_name = model.__name__
        
        lines.append(f'    @migrator.create_model')
        lines.append(f'    class {model_name}(pw.Model):')
        
        # Get all fields
        fields = model._meta.fields
        for field_name, field in fields.items():
            field_code = generate_field_code(field, field_name)
            lines.append(f'        {field_name} = {field_code}')
        
        lines.append('')
        lines.append('        class Meta:')
        lines.append(f'            table_name = "{table_name}"')
        
        # Add indexes if defined
        indexes = getattr(model._meta, 'indexes', None)
        if indexes:
            lines.append(f'            indexes = {indexes}')
        
        lines.append('')
    
    # Generate SQL for adding new fields to existing tables
    for table_name, changes in field_changes.items():
        if changes.get('added'):
            for field_name, field in changes['added'].items():
                sql, index_sql = generate_add_field_sql(table_name, field, field_name)
                lines.append(f'    migrator.sql("{sql}")')
                if index_sql:
                    lines.append(f'    migrator.sql("{index_sql}")')
                lines.append('')
    
    # Generate SQL for modifying fields in existing tables
    for table_name, changes in field_changes.items():
        if changes.get('changed'):
            for field_name, (old_info, field) in changes['changed'].items():
                modify_sql = generate_modify_field_sql(table_name, field, field_name)
                lines.append(f'    migrator.sql("{modify_sql}")')
                lines.append('')
    
    # Generate rollback
    lines.append('')
    lines.append('def rollback(migrator: Migrator, database: pw.Database, *, fake=False):')
    lines.append('    """Write your rollback migrations here."""')
    lines.append('')
    
    # Rollback: reverse field type changes first (before removing added fields)
    for table_name, changes in field_changes.items():
        if changes.get('changed'):
            for field_name, (old_info, field) in changes['changed'].items():
                rollback_modify_sql = generate_rollback_modify_sql(table_name, old_info, field_name)
                lines.append(f'    # Note: Data values may need manual handling if type conversion caused data loss')
                lines.append(f'    migrator.sql("{rollback_modify_sql}")')
    
    # Rollback: remove added fields using SQL
    for table_name, changes in field_changes.items():
        if changes.get('added'):
            for field_name in changes['added'].keys():
                rollback_sql = generate_rollback_field_sql(table_name, field_name)
                lines.append(f'    migrator.sql("{rollback_sql}")')
    
    # Rollback: remove tables (in reverse order)
    for model in reversed(new_tables):
        table_name = model._meta.table_name
        lines.append(f'    migrator.remove_model("{table_name}")')
    
    lines.append('')
    
    return '\n'.join(lines)


def create_migration(router: Router, models: list, db, name: str = "auto"):
    """Create a new migration by auto-detecting model changes
    
    Detects:
    1. New tables -> generate create_model
    2. New fields in existing tables -> generate add_fields
    3. Field type changes -> generate change_fields
    
    Args:
        router: peewee-migrate Router instance
        models: List of model classes to compare against database
        db: Database connection
        name: Migration name
    """
    try:
        # Get existing tables from database
        cursor = db.execute_sql(
            "SELECT table_name FROM information_schema.tables WHERE table_schema = %s",
            (db.database,)
        )
        existing_tables = {row[0] for row in cursor.fetchall()}
        
        new_tables = []
        field_changes = {}
        
        for model in models:
            table_name = model._meta.table_name
            
            if table_name not in existing_tables:
                # New table
                new_tables.append(model)
                logger.info(f"New table detected: {table_name}")
            else:
                # Existing table - check for field changes
                logger.info(f"Checking existing table: {table_name}")
                
                # Get model fields (exclude auto-generated)
                model_fields = {}
                for field_name, field in model._meta.fields.items():
                    # Skip id and base model fields
                    if field_name in ('id', 'create_time', 'create_date', 'update_time', 'update_date'):
                        continue
                    if hasattr(field, '_auto_created') and field._auto_created:
                        continue
                    model_fields[field_name] = field
                
                # Get database columns
                db_columns = get_table_columns(db, table_name)
                
                # Compare
                changes = compare_fields(model_fields, db_columns)
                
                if changes['added'] or changes['changed']:
                    field_changes[table_name] = changes
        
        # Check if any changes detected
        if not new_tables and not field_changes:
            logger.info("No schema changes detected, migration not created")
            return None
        
        # Generate migration file content
        migration_content = generate_migration_content(new_tables, field_changes, router.migrate_dir, name)
        
        # Get next migration number (count existing migration files)
        existing_migrations = [f for f in os.listdir(router.migrate_dir) if f.endswith('.py') and not f.startswith('_')]
        migration_num = len(existing_migrations) + 1
        migration_file = os.path.join(router.migrate_dir, f'{migration_num:03d}_{name}.py')
        
        with open(migration_file, 'w') as f:
            f.write(migration_content)
        
        logger.info(f"Created migration: {migration_file}")
        return migration_file
        
    except Exception as e:
        logger.error(f"Failed to create migration: {e}")
        raise


def run_migrations(router: Router):
    """Run all pending migrations"""
    try:
        diff = router.diff
        if not diff:
            logger.info("No pending migrations to run")
            return
        
        router.run()
        logger.info("Migrations completed successfully")
    except Exception as e:
        logger.error(f"Failed to run migrations: {e}")
        raise


def list_migrations(router: Router):
    """List all migrations"""
    todo = router.todo
    if not todo:
        logger.info("No migration files found")
        return
    
    logger.info("Available migrations:")
    done = set(router.done)
    for migration in todo:
        status = "applied" if migration in done else "pending"
        logger.info(f"  [{status}] {migration}")


def diff_schema(models: list, db):
    """Show schema differences between models and database"""
    logger.info("Checking schema differences...")
    
    # Tables to ignore (managed by peewee-migrate)
    IGNORE_TABLES = {'migratehistory'}
    
    # Get all model table names
    model_tables = set()
    for model in models:
        table_name = model._meta.table_name
        model_tables.add(table_name)
    
    logger.info(f"Found {len(model_tables)} model tables")
    
    # Get existing tables from database
    cursor = db.execute_sql(
        "SELECT table_name FROM information_schema.tables WHERE table_schema = %s",
        (db.database,)
    )
    existing_tables = {row[0] for row in cursor.fetchall() if row[0] not in IGNORE_TABLES}
    
    # Find tables that exist in models but not in database
    missing_tables = model_tables - existing_tables
    if missing_tables:
        logger.warning(f"Tables not in database ({len(missing_tables)}): {', '.join(sorted(missing_tables))}")
    
    # Find tables that exist in database but not in models
    extra_tables = existing_tables - model_tables
    if extra_tables:
        logger.info(f"Tables in database but not in models: {', '.join(sorted(extra_tables))}")
    
    # Check field differences for existing tables
    common_tables = model_tables & existing_tables
    if common_tables:
        logger.info(f"\nChecking field differences for {len(common_tables)} existing tables...")
        
        total_added = 0
        total_changed = 0
        
        for model in models:
            table_name = model._meta.table_name
            if table_name not in common_tables:
                continue
            
            # Get model fields
            model_fields = {}
            for field_name, field in model._meta.fields.items():
                if field_name in ('id', 'create_time', 'create_date', 'update_time', 'update_date'):
                    continue
                model_fields[field_name] = field
            
            # Get database columns
            db_columns = get_table_columns(db, table_name)
            
            # Compare
            changes = compare_fields(model_fields, db_columns)
            
            if changes['added']:
                total_added += len(changes['added'])
                field_details = [f"{k}:{v.__class__.__name__}" for k, v in changes['added'].items()]
                logger.info(f"  {table_name}: {len(changes['added'])} new field(s) - {field_details}")
            
            if changes['changed']:
                total_changed += len(changes['changed'])
                field_details = [f"{k}:{v[1].__class__.__name__}" for k, v in changes['changed'].items()]
                logger.info(f"  {table_name}: {len(changes['changed'])} changed field(s) - {field_details}")
        
        logger.info(f"\nSummary: {total_added} new fields, {total_changed} changed fields")


def main():
    parser = argparse.ArgumentParser(
        description='Database Schema Synchronization Tool using peewee-migrate',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # List all migrations
  python db_schema_sync.py --list --host localhost --port 3306 --user root --password xxx --database rag_flow --version v0.24.0
  
  # Create migration from model changes
  python db_schema_sync.py --create --host localhost --port 3306 --user root --password xxx --database rag_flow --version v0.24.0
  
  # Run all pending migrations
  python db_schema_sync.py --migrate --host localhost --port 3306 --user root --password xxx --database rag_flow --version v0.24.0
  
  # Show schema differences
  python db_schema_sync.py --diff --host localhost --port 3306 --user root --password xxx --database rag_flow --version v0.24.0
"""
    )
    
    # Database connection options
    parser.add_argument('--host', type=str, required=True, help='MySQL host')
    parser.add_argument('--port', type=int, default=3306, help='MySQL port (default: 3306)')
    parser.add_argument('--user', type=str, required=True, help='MySQL user')
    parser.add_argument('--password', type=str, required=True, help='MySQL password')
    parser.add_argument('--database', type=str, required=True, help='MySQL database name')
    
    # Version option
    parser.add_argument('--version', '-v', type=str, required=True, 
                       help='Version number in format vxx.xx.xx (e.g., v0.24.0)')
    
    # Action options
    parser.add_argument('--list', '-l', action='store_true', help='List all migrations')
    parser.add_argument('--create', '-c', action='store_true', 
                       help='Create migration from model changes (auto-detect)')
    parser.add_argument('--migrate', '-m', action='store_true', help='Run pending migrations')
    parser.add_argument('--diff', '-d', action='store_true', help='Show schema differences')
    
    # Migration options
    parser.add_argument('--name', '-n', type=str, default='auto', help='Migration name')
    
    args = parser.parse_args()
    
    # Validate version format
    if not validate_version(args.version):
        logger.error(f"Invalid version format: {args.version}. Expected format: vxx.xx.xx (e.g., v0.24.0)")
        sys.exit(1)
    
    # Validate at least one action is specified
    if not any([args.list, args.create, args.migrate, args.diff]):
        parser.print_help()
        logger.error("Please specify at least one action: --list, --create, --migrate, or --diff")
        sys.exit(1)
    
    # Convert version to directory name
    version_dir = version_to_dirname(args.version)
    migrate_dir = os.path.join(PROJECT_BASE, 'tools', 'migrate', version_dir)
    
    logger.info(f"Version: {args.version}")
    logger.info(f"Migration directory: {migrate_dir}")
    
    # Create migration directory if it doesn't exist
    os.makedirs(migrate_dir, exist_ok=True)
    
    # Load database models
    logger.info("Loading database models from api/db/db_models.py...")
    models, _ = load_db_models()
    logger.info(f"Found {len(models)} model classes")
    
    # Create database connection
    db = create_database_connection(
        host=args.host,
        port=args.port,
        user=args.user,
        password=args.password,
        database=args.database
    )
    
    try:
        db.connect()
        logger.info(f"Connected to database: {args.database}")
        
        # Create router
        router = Router(
            db,
            migrate_dir,
            ignore=['basemodel', 'base_model', 'migratehistory']
        )
        
        # Execute requested actions
        if args.list:
            list_migrations(router)
        
        if args.create:
            create_migration(router, models, db, args.name)
        
        if args.migrate:
            run_migrations(router)
        
        if args.diff:
            diff_schema(models, db)
    
    finally:
        if not db.is_closed():
            db.close()
            logger.info("Database connection closed")
    
    logger.info("Done.")


if __name__ == '__main__':
    main()