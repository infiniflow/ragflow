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

from peewee import MySQLDatabase, Model
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


def create_migration(router: Router, name: str = "auto", auto: bool = False, models: list = None):
    """Create a new migration
    
    Args:
        router: peewee-migrate Router instance
        name: Migration name
        auto: If True, auto-generate migration SQL from model changes
        models: List of model classes to compare against database
    """
    try:
        if auto and models:
            # Auto-generate migration SQL by comparing models with database
            migration_name = router.create(name, auto=models)
            if migration_name:
                logger.info(f"Created auto-generated migration: {migration_name}")
            else:
                logger.info("No schema changes detected, migration not created")
            return migration_name
        else:
            # Create empty migration template
            migration_name = router.create(name)
            logger.info(f"Created migration template: {migration_name}")
            return migration_name
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
    
    # Get all model table names
    model_tables = set()
    for model in models:
        table_name = model._meta.table_name
        model_tables.add(table_name)
    
    logger.info(f"Found {len(model_tables)} model tables: {', '.join(sorted(model_tables))}")
    
    # Get existing tables from database
    cursor = db.execute_sql(
        "SELECT table_name FROM information_schema.tables WHERE table_schema = %s",
        (db.database,)
    )
    existing_tables = {row[0] for row in cursor.fetchall()}
    
    # Find tables that exist in models but not in database
    missing_tables = model_tables - existing_tables
    if missing_tables:
        logger.warning(f"Tables not in database: {', '.join(sorted(missing_tables))}")
    
    # Find tables that exist in database but not in models
    extra_tables = existing_tables - model_tables
    if extra_tables:
        logger.info(f"Tables in database but not in models: {', '.join(sorted(extra_tables))}")
    
    # Tables in both
    common_tables = model_tables & existing_tables
    if common_tables:
        logger.info(f"Tables in both: {len(common_tables)}")


def main():
    parser = argparse.ArgumentParser(
        description='Database Schema Synchronization Tool using peewee-migrate',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # List all migrations
  python db_schema_sync.py --list --host localhost --port 3306 --user root --password xxx --database rag_flow --version v0.24.0
  
  # Create an empty migration template
  python db_schema_sync.py --create --name add_user_table --host localhost --port 3306 --user root --password xxx --database rag_flow --version v0.24.0
  
  # Auto-generate migration from model changes
  python db_schema_sync.py --create --auto --host localhost --port 3306 --user root --password xxx --database rag_flow --version v0.24.0
  
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
    parser.add_argument('--create', action='store_true', help='Create a new migration')
    parser.add_argument('--auto', '-a', action='store_true', 
                       help='Auto-generate migration SQL from model changes (use with --create)')
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
    
    # Validate --auto requires --create
    if args.auto and not args.create:
        logger.error("--auto can only be used with --create")
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
            ignore=['basemodel', 'base_model']
        )
        
        # Execute requested actions
        if args.list:
            list_migrations(router)
        
        if args.create:
            create_migration(router, args.name, auto=args.auto, models=models if args.auto else None)
        
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