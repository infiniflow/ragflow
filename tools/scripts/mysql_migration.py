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
MySQL Data Migration Script

This script provides a flexible MySQL data migration tool that supports:
1. MySQL configuration via config file or command line arguments
2. Direct peewee operations without importing api.db.services
3. Configurable migration stages via command line
4. Migration logging with table names, row counts, and duration
"""

import argparse
import logging
import os
import sys
import time
import uuid

from peewee import (
    CharField,
    IntegerField,
    BigIntegerField,
    DateTimeField,
    MySQLDatabase,
    Model,
    PrimaryKeyField,
    TextField,
)
from playhouse.migrate import MySQLMigrator

# Add project root to path for imports
PROJECT_BASE = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.path.insert(0, PROJECT_BASE)

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class MigrationConfig:
    """Configuration for MySQL connection"""
    
    def __init__(self, host: str = 'localhost', port: int = 3306, 
                 user: str = 'root', password: str = '', database: str = 'rag_flow'):
        self.host = host
        self.port = port
        self.user = user
        self.password = password
        self.database = database
    
    @classmethod
    def from_config_file(cls, config_path: str) -> 'MigrationConfig':
        """Load configuration from YAML config file"""
        try:
            from ruamel.yaml import YAML
            yaml = YAML(typ="safe", pure=True)
            
            with open(config_path, 'r') as f:
                config = yaml.load(f)
            
            # Try to get database config
            db_config = config.get('database', config.get('mysql', {}))
            
            return cls(
                host=db_config.get('host', 'localhost'),
                port=db_config.get('port', 3306),
                user=db_config.get('user', 'root'),
                password=db_config.get('password', ''),
                database=db_config.get('name', db_config.get('database', 'rag_flow'))
            )
        except Exception as e:
            logger.warning(f"Failed to load config file: {e}, using defaults")
            return cls()


class MigrationStats:
    """Track migration statistics"""
    
    def __init__(self):
        self.tables_operated = []
        self.rows_processed = 0
        self.start_time = None
        self.end_time = None
        self.stage_stats = []
    
    def start(self):
        self.start_time = time.time()
    
    def end(self):
        self.end_time = time.time()
    
    def add_stage_stats(self, stage_name: str, tables: list, rows: int, duration: float):
        self.stage_stats.append({
            'stage': stage_name,
            'tables': tables,
            'rows': rows,
            'duration': duration
        })
        self.tables_operated.extend(tables)
        self.rows_processed += rows
    
    def print_summary(self):
        duration = self.end_time - self.start_time if self.end_time and self.start_time else 0
        logger.info("=" * 60)
        logger.info("Migration Summary")
        logger.info("=" * 60)
        logger.info(f"Total Duration: {duration:.2f}s")
        logger.info(f"Total Rows Processed: {self.rows_processed}")
        logger.info(f"Tables Operated: {', '.join(set(self.tables_operated))}")
        logger.info("-" * 60)
        logger.info("Stage Details:")
        for stat in self.stage_stats:
            logger.info(f"  [{stat['stage']}] Tables: {', '.join(stat['tables'])}, "
                       f"Rows: {stat['rows']}, Duration: {stat['duration']:.2f}s")
        logger.info("=" * 60)


class MigrationDatabase:
    """Database wrapper for migrations"""
    
    def __init__(self, config: MigrationConfig):
        self.config = config
        self.db = MySQLDatabase(
            config.database,
            host=config.host,
            port=config.port,
            user=config.user,
            password=config.password,
            charset='utf8mb4'
        )
        self.migrator = MySQLMigrator(self.db)
    
    def connect(self):
        self.db.connect()
        logger.info(f"Connected to MySQL database: {self.config.database}")
    
    def close(self):
        if not self.db.is_closed():
            self.db.close()
            logger.info("Database connection closed")
    
    def execute_sql(self, sql: str, params=None):
        return self.db.execute_sql(sql, params)
    
    def table_exists(self, table_name: str) -> bool:
        cursor = self.execute_sql(
            "SELECT COUNT(*) FROM information_schema.tables "
            "WHERE table_schema = %s AND table_name = %s",
            (self.config.database, table_name)
        )
        return cursor.fetchone()[0] > 0


# Define model classes for migration (not importing from api.db.db_models)
class BaseModel(Model):
    """Base model for migration tables"""
    create_time = BigIntegerField(null=True, index=True)
    create_date = DateTimeField(null=True, index=True)
    update_time = BigIntegerField(null=True, index=True)
    update_date = DateTimeField(null=True, index=True)
    
    class Meta:
        database = None  # Will be set dynamically


class TenantLLM(BaseModel):
    """Tenant LLM model (source table)"""
    id = PrimaryKeyField()
    tenant_id = CharField(max_length=32, null=False, index=True)
    llm_factory = CharField(max_length=128, null=False, index=True)
    model_type = CharField(max_length=128, null=True, index=True)
    llm_name = CharField(max_length=128, null=True, default="", index=True)
    api_key = TextField(null=True)
    api_base = CharField(max_length=255, null=True)
    max_tokens = IntegerField(default=8192, index=True)
    used_tokens = IntegerField(default=0, index=True)
    status = CharField(max_length=1, null=False, default="1", index=True)
    
    class Meta:
        table_name = "tenant_llm"
        database = None


class TenantModelProvider(BaseModel):
    """Tenant Model Provider model (target table)"""
    id = CharField(max_length=32, primary_key=True)
    provider_name = CharField(max_length=128, null=False, index=True)
    tenant_id = CharField(max_length=32, null=False, index=True)
    
    class Meta:
        table_name = "tenant_model_provider"
        database = None


class MigrationStage:
    """Base class for migration stages"""
    
    name = "base_stage"
    description = "Base migration stage"
    source_tables = []
    target_tables = []
    
    def __init__(self, db: MigrationDatabase, dry_run: bool = True, create_table_only: bool = False):
        self.db = db
        self.dry_run = dry_run
        self.create_table_only = create_table_only
    
    def check(self) -> bool:
        """Check if migration is needed"""
        raise NotImplementedError
    
    def execute(self) -> tuple[int, list]:
        """Execute migration, returns (rows_affected, tables_operated)"""
        raise NotImplementedError
    
    def create_target_table(self):
        """Create target table (override in subclass if needed)"""
        pass


class TenantModelProviderStage(MigrationStage):
    """Migrate tenant_llm to tenant_model_provider"""
    
    name = "tenant_model_provider"
    description = "Migrate tenant_llm.llm_factory to tenant_model_provider.provider_name"
    source_tables = ["tenant_llm"]
    target_tables = ["tenant_model_provider"]
    
    def current_timestamp(self) -> int:
        return int(time.time())
    
    def generate_uuid(self) -> str:
        """Generate 32-character UUID1"""
        return uuid.uuid1().hex
    
    def check(self) -> bool:
        """Check if migration is needed"""
        # Check if source table exists
        if not self.db.table_exists("tenant_llm"):
            logger.warning("Source table 'tenant_llm' does not exist")
            return False
        
        # Check if target table exists
        if not self.db.table_exists("tenant_model_provider"):
            if self.dry_run:
                logger.info("[DRY RUN] Target table 'tenant_model_provider' does not exist. "
                           "Use --execute to create and populate the table.")
                return False
            logger.info("Target table 'tenant_model_provider' does not exist, will create")
            return True
        
        # Check if there's data to migrate
        cursor = self.db.execute_sql(
            "SELECT COUNT(*) FROM tenant_llm t1 "
            "WHERE NOT EXISTS ("
            "  SELECT 1 FROM tenant_model_provider t2 "
            "  WHERE t2.tenant_id = t1.tenant_id AND t2.provider_name = t1.llm_factory"
            ")"
        )
        count = cursor.fetchone()[0]
        
        if count == 0:
            logger.info("No new data to migrate from tenant_llm to tenant_model_provider")
            return False
        
        logger.info(f"Found {count} rows to migrate from tenant_llm to tenant_model_provider")
        return True
    
    def execute(self) -> tuple[int, list]:
        """Execute migration"""
        current_ts = self.current_timestamp()
        rows_inserted = 0
        
        # Check if target table exists
        if not self.db.table_exists("tenant_model_provider"):
            if self.dry_run:
                logger.info("[DRY RUN] Target table 'tenant_model_provider' does not exist. "
                           "Use --execute to create and populate the table.")
                return 0, []
            logger.info("Target table 'tenant_model_provider' does not exist, will create")
            self.create_target_table()
        
        # If create_table_only mode, skip data migration
        if self.create_table_only:
            logger.info("[CREATE TABLE ONLY] Target table created/verified, skipping data migration")
            return 0, self.target_tables
        
        # Get distinct tenant_id, llm_factory pairs that don't exist in target
        cursor = self.db.execute_sql(
            "SELECT DISTINCT tenant_id, llm_factory FROM tenant_llm t1 "
            "WHERE NOT EXISTS ("
            "  SELECT 1 FROM tenant_model_provider t2 "
            "  WHERE t2.tenant_id = t1.tenant_id AND t2.provider_name = t1.llm_factory"
            ")"
        )
        
        records = cursor.fetchall()
        
        if not records:
            logger.info("No records to migrate")
            return 0, []
        
        logger.info(f"Migrating {len(records)} unique tenant_id/llm_factory pairs...")
        
        if self.dry_run:
            logger.info(f"[DRY RUN] Would insert {len(records)} records")
            return len(records), self.target_tables
        
        # Insert records in batches
        batch_size = 100
        for i in range(0, len(records), batch_size):
            batch = records[i:i + batch_size]
            values = []
            for tenant_id, llm_factory in batch:
                record_id = self.generate_uuid()
                values.append(f"('{record_id}', '{llm_factory}', '{tenant_id}', "
                            f"{current_ts}, FROM_UNIXTIME({current_ts}), "
                            f"{current_ts}, FROM_UNIXTIME({current_ts}))")
            
            insert_sql = f"""
                INSERT INTO tenant_model_provider 
                (id, provider_name, tenant_id, create_time, create_date, update_time, update_date)
                VALUES {', '.join(values)}
            """
            self.db.execute_sql(insert_sql)
            rows_inserted += len(batch)
            logger.info(f"Inserted batch {i // batch_size + 1}: {len(batch)} records")
        
        return rows_inserted, self.target_tables
    
    def create_target_table(self):
        """Create tenant_model_provider table"""
        create_sql = """
        CREATE TABLE IF NOT EXISTS tenant_model_provider (
            id VARCHAR(32) NOT NULL PRIMARY KEY,
            provider_name VARCHAR(128) NOT NULL,
            tenant_id VARCHAR(32) NOT NULL,
            create_time BIGINT,
            create_date DATETIME,
            update_time BIGINT,
            update_date DATETIME,
            INDEX idx_provider_name (provider_name),
            INDEX idx_tenant_id (tenant_id),
            UNIQUE INDEX idx_tenant_provider_unique (tenant_id, provider_name)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
        """
        self.db.execute_sql(create_sql)
        logger.info("Created tenant_model_provider table")


# Registry of available migration stages
MIGRATION_STAGES = {
    'tenant_model_provider': TenantModelProviderStage,
}


def list_available_stages():
    """List all available migration stages"""
    logger.info("Available migration stages:")
    for name, stage_cls in MIGRATION_STAGES.items():
        logger.info(f"  - {name}: {stage_cls.description}")
        logger.info(f"    Source tables: {stage_cls.source_tables}")
        logger.info(f"    Target tables: {stage_cls.target_tables}")


def run_migration(config: MigrationConfig, stages: list, dry_run: bool = True, 
                  create_table_only: bool = False):
    """Run migration with specified stages"""
    stats = MigrationStats()
    stats.start()
    
    db = MigrationDatabase(config)
    
    try:
        db.connect()
        
        total_stages = len(stages)
        
        for idx, stage_name in enumerate(stages, 1):
            logger.info(f"\n{'=' * 60}")
            logger.info(f"Stage [{idx}/{total_stages}]: {stage_name}")
            logger.info(f"{'=' * 60}")
            
            if stage_name not in MIGRATION_STAGES:
                logger.error(f"Unknown stage: {stage_name}")
                stats.add_stage_stats(stage_name, [], 0, 0)
                continue
            
            stage_cls = MIGRATION_STAGES[stage_name]
            stage = stage_cls(db, dry_run=dry_run, create_table_only=create_table_only)
            
            stage_start = time.time()
            
            # For create_table_only mode, skip check and directly execute
            if create_table_only:
                logger.info("[CREATE TABLE ONLY] Skipping check, will create/verify target table")
                rows, tables = stage.execute()
            else:
                # Check if migration is needed
                if not stage.check():
                    logger.info(f"Stage '{stage_name}' check: no migration needed")
                    stats.add_stage_stats(stage_name, [], 0, time.time() - stage_start)
                    continue
                
                # Execute migration
                rows, tables = stage.execute()
            
            stage_duration = time.time() - stage_start
            
            stats.add_stage_stats(stage_name, tables, rows, stage_duration)
            logger.info(f"Stage '{stage_name}' completed: {rows} rows in {stage_duration:.2f}s")
        
    finally:
        db.close()
        stats.end()
        stats.print_summary()


def main():
    parser = argparse.ArgumentParser(
        description='MySQL Data Migration Tool',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # List available stages
  python mysql_migration.py --list-stages
  
  # Dry run (default - check only, no write)
  python mysql_migration.py --stages tenant_model_provider --config /path/to/config.yaml
  
  # Create target tables only (no data migration)
  python mysql_migration.py --stages tenant_model_provider --config /path/to/config.yaml --create-table-only
  
  # Execute full migration (create tables and migrate data)
  python mysql_migration.py --stages tenant_model_provider --config /path/to/config.yaml --execute
  
  # Run multiple stages
  python mysql_migration.py --stages stage1,stage2,stage3 --config /path/to/config.yaml --execute
"""
    )
    
    # Configuration options
    parser.add_argument('--config', '-c', type=str, help='Path to YAML config file')
    
    # Migration options
    parser.add_argument('--stages', '-s', type=str, help='Comma-separated list of stages to run')
    parser.add_argument('--list-stages', '-l', action='store_true', help='List available stages')
    parser.add_argument('--execute', '-e', action='store_true', default=False,
                       help='Execute full migration: create tables and migrate data')
    parser.add_argument('--create-table-only', action='store_true', default=False,
                       help='Only create target tables, skip data migration')
    
    args = parser.parse_args()
    
    # List stages and exit
    if args.list_stages:
        list_available_stages()
        return
    
    # Parse stages
    if not args.stages:
        logger.error("No stages specified. Use --stages to specify stages or --list-stages to see available stages.")
        sys.exit(1)
    
    stages = [s.strip() for s in args.stages.split(',')]
    
    # Load configuration
    if args.config:
        config = MigrationConfig.from_config_file(args.config)
    else:
        logging.error("No config file specified. Use --config to specify config file.")
        sys.exit(1)
    
    logger.info(f"MySQL Configuration: host={config.host}, port={config.port}, "
               f"user={config.user}, database={config.database}")
    
    # Three mutually exclusive modes: dry-run (default), create-table-only, execute
    if args.execute and args.create_table_only:
        logger.error("--execute and --create-table-only are mutually exclusive")
        sys.exit(1)
    
    dry_run = True
    create_table_only = False
    
    if args.create_table_only:
        logger.info("Running in CREATE TABLE ONLY mode (create tables, no data migration)")
        dry_run = False
        create_table_only = True
    elif args.execute:
        logger.info("Running in EXECUTE mode (create tables and migrate data)")
        dry_run = False
    else:
        logger.info("Running in DRY-RUN mode (check only, no write). "
                   "Use --create-table-only to create tables, or --execute for full migration.")
    
    run_migration(
        config=config,
        stages=stages,
        dry_run=dry_run,
        create_table_only=create_table_only
    )


if __name__ == '__main__':
    main()
