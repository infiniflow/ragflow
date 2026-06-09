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
import json
import logging
import os
import sys
import time
import uuid

from packaging.version import InvalidVersion, Version
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


MIGRATION_DB_VERSION_MARKER = "mysql_migration.database.version"


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

    def column_exists(self, table_name: str, column_name: str) -> bool:
        cursor = self.execute_sql(
            "SELECT COUNT(*) FROM information_schema.columns "
            "WHERE table_schema = %s AND table_name = %s AND column_name = %s",
            (self.config.database, table_name, column_name)
        )
        return cursor.fetchone()[0] > 0

    def get_system_setting_value(self, name: str) -> str | None:
        if not self.table_exists("system_settings"):
            logger.info("Table 'system_settings' does not exist, migration marker is unavailable")
            return None
        cursor = self.execute_sql(
            "SELECT `value` FROM `system_settings` WHERE `name` = %s",
            (name,),
        )
        row = cursor.fetchone()
        return row[0] if row else None

    def upsert_system_setting(self, name: str, value: str, source: str = "migration", data_type: str = "string"):
        if not self.table_exists("system_settings"):
            logger.warning("Table 'system_settings' does not exist, migration marker was not saved")
            return

        current_ts = int(time.time())
        self.execute_sql(
            """
            INSERT INTO `system_settings`
            (`name`, `source`, `data_type`, `value`, `create_time`, `create_date`, `update_time`, `update_date`)
            VALUES (%s, %s, %s, %s, %s, FROM_UNIXTIME(%s), %s, FROM_UNIXTIME(%s))
            ON DUPLICATE KEY UPDATE
              `source` = VALUES(`source`),
              `data_type` = VALUES(`data_type`),
              `value` = VALUES(`value`),
              `update_time` = VALUES(`update_time`),
              `update_date` = VALUES(`update_date`)
            """,
            (
                name,
                source,
                data_type,
                value,
                current_ts * 1000,
                current_ts,
                current_ts * 1000,
                current_ts,
            ),
        )

    def get_database_version(self) -> str | None:
        return self.get_system_setting_value(MIGRATION_DB_VERSION_MARKER)

    def set_database_version(self, version: str):
        self.upsert_system_setting(MIGRATION_DB_VERSION_MARKER, version)


def parse_migration_version(version: str | None) -> Version | None:
    if not version:
        return None
    normalized = version.strip()
    if normalized.startswith(("v", "V")):
        normalized = normalized[1:]
    try:
        return Version(normalized)
    except InvalidVersion:
        logger.warning("Invalid migration version format: %s", version)
        return None


def should_skip_migration(current_db_version: str | None, target_version: str) -> bool:
    current = parse_migration_version(current_db_version)
    target = parse_migration_version(target_version)
    if current is None or target is None:
        return False
    return current >= target


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
        self._noop_completes_migration = False
    
    def check(self) -> bool:
        """Check if migration is needed"""
        raise NotImplementedError
    
    def execute(self) -> tuple[int, list]:
        """Execute migration, returns (rows_affected, tables_operated)"""
        raise NotImplementedError
    
    def create_target_table(self):
        """Create target table (override in subclass if needed)"""
        pass

    def mark_noop_completes_migration(self):
        self._noop_completes_migration = True

    def noop_completes_migration(self) -> bool:
        return self._noop_completes_migration


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
            self.mark_noop_completes_migration()
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
        
        # Insert records in batches with parameterized SQL to avoid quote breakage/injection
        batch_size = 100
        for i in range(0, len(records), batch_size):
            batch = records[i:i + batch_size]
            placeholders = []
            params = []
            for tenant_id, llm_factory in batch:
                record_id = self.generate_uuid()
                placeholders.append("(%s, %s, %s, %s, FROM_UNIXTIME(%s), %s, FROM_UNIXTIME(%s))")
                params.extend([
                    record_id,
                    llm_factory,
                    tenant_id,
                    current_ts * 1000,
                    current_ts,
                    current_ts * 1000,
                    current_ts,
                ])
            insert_sql = f"""
                INSERT INTO tenant_model_provider 
                (id, provider_name, tenant_id, create_time, create_date, update_time, update_date)
                VALUES {', '.join(placeholders)}
            """
            self.db.execute_sql(insert_sql, params)
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


class TenantModelInstanceStage(MigrationStage):
    """Migrate tenant_llm to tenant_model_instance"""

    name = "tenant_model_instance"
    description = "Migrate tenant_llm to tenant_model_instance with provider_id lookup"
    source_tables = ["tenant_llm", "tenant_model_provider"]
    target_tables = ["tenant_model_instance"]

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

        # Check if tenant_model_provider exists (dependency)
        if not self.db.table_exists("tenant_model_provider"):
            if self.dry_run:
                logger.info("[DRY RUN] Dependency table 'tenant_model_provider' does not exist. "
                           "Run 'tenant_model_provider' stage first or use --execute.")
                return False
            logger.warning("Dependency table 'tenant_model_provider' does not exist. "
                          "Please run 'tenant_model_provider' stage first.")
            return False

        # Check if target table exists
        if not self.db.table_exists("tenant_model_instance"):
            if self.dry_run:
                logger.info("[DRY RUN] Target table 'tenant_model_instance' does not exist. "
                           "Use --execute to create and populate the table.")
                return False
            logger.info("Target table 'tenant_model_instance' does not exist, will create")
            return True

        # Check if there's data to migrate (distinct by tenant_id, llm_factory, api_key)
        cursor = self.db.execute_sql(
            "SELECT COUNT(*) FROM ("
            "  SELECT tl.tenant_id, tl.llm_factory, tl.api_key, tmp.id as provider_id "
            "  FROM tenant_llm tl "
            "  INNER JOIN tenant_model_provider tmp ON tmp.tenant_id = tl.tenant_id AND tmp.provider_name = tl.llm_factory "
            "  WHERE NOT EXISTS ("
            "    SELECT 1 FROM tenant_model_instance tmi "
            "    WHERE tmi.provider_id = tmp.id AND tmi.api_key = tl.api_key"
            "  ) "
            "  GROUP BY tl.tenant_id, tl.llm_factory, tl.api_key, tmp.id"
            ") AS distinct_records"
        )
        count = cursor.fetchone()[0]

        if count == 0:
            self.mark_noop_completes_migration()
            logger.info("No new data to migrate from tenant_llm to tenant_model_instance")
            return False

        logger.info(f"Found {count} rows to migrate from tenant_llm to tenant_model_instance")
        return True

    def execute(self) -> tuple[int, list]:
        """Execute migration"""
        current_ts = self.current_timestamp()
        rows_inserted = 0

        # Check if tenant_model_provider exists (dependency)
        if not self.db.table_exists("tenant_model_provider"):
            logger.error("Dependency table 'tenant_model_provider' does not exist. "
                        "Please run 'tenant_model_provider' stage first.")
            return 0, []

        # Check if target table exists
        if not self.db.table_exists("tenant_model_instance"):
            if self.dry_run:
                logger.info("[DRY RUN] Target table 'tenant_model_instance' does not exist. "
                           "Use --execute to create and populate the table.")
                return 0, []
            logger.info("Target table 'tenant_model_instance' does not exist, will create")
            self.create_target_table()

        # If create_table_only mode, skip data migration
        if self.create_table_only:
            logger.info("[CREATE TABLE ONLY] Target table created/verified, skipping data migration")
            return 0, self.target_tables

        # Get records from tenant_llm with provider_id lookup
        # Group by tenant_id, llm_factory, api_key to get distinct records
        # instance_name = llm_factory, provider_id from tenant_model_provider, api_key from tenant_llm
        cursor = self.db.execute_sql(
            "SELECT tl.tenant_id, tl.llm_factory, tl.api_key, MAX(tl.status) as status, tmp.id as provider_id "
            "FROM tenant_llm tl "
            "INNER JOIN tenant_model_provider tmp ON tmp.tenant_id = tl.tenant_id AND tmp.provider_name = tl.llm_factory "
            "WHERE NOT EXISTS ("
            "  SELECT 1 FROM tenant_model_instance tmi "
            "  WHERE tmi.provider_id = tmp.id AND tmi.api_key = tl.api_key"
            ") "
            "GROUP BY tl.tenant_id, tl.llm_factory, tl.api_key, tmp.id"
        )

        records = cursor.fetchall()

        if not records:
            logger.info("No records to migrate")
            return 0, []

        logger.info(f"Migrating {len(records)} tenant_model_instance records...")

        if self.dry_run:
            logger.info(f"[DRY RUN] Would insert {len(records)} records")
            for tenant_id, llm_factory, api_key, status, provider_id in records[:5]:
                logger.info(f"  instance_name=default, provider_id={provider_id}, api_key=***")
            if len(records) > 5:
                logger.info(f"  ... and {len(records) - 5} more records")
            return len(records), self.target_tables

        # Insert records in batches
        batch_size = 100
        for i in range(0, len(records), batch_size):
            batch = records[i:i + batch_size]
            values = []
            for tenant_id, llm_factory, api_key, status, provider_id in batch:
                record_id = self.generate_uuid()
                instance_name = "default"
                api_key_escaped = api_key.replace("'", "''") if api_key else ""
                status_val = "active" if status in ["1", "active", "enable"] else "inactive"
                values.append(f"('{record_id}', '{instance_name}', '{provider_id}', "
                            f"'{api_key_escaped}', '{status_val}', "
                            f"{current_ts * 1000}, FROM_UNIXTIME({current_ts}), "
                            f"{current_ts * 1000}, FROM_UNIXTIME({current_ts}))")

            insert_sql = f"""
                INSERT INTO tenant_model_instance 
                (id, instance_name, provider_id, api_key, status, create_time, create_date, update_time, update_date)
                VALUES {', '.join(values)}
            """
            self.db.execute_sql(insert_sql)
            rows_inserted += len(batch)
            logger.info(f"Inserted batch {i // batch_size + 1}: {len(batch)} records")

        return rows_inserted, self.target_tables

    def create_target_table(self):
        """Create tenant_model_instance table"""
        create_sql = """
        CREATE TABLE IF NOT EXISTS tenant_model_instance (
            id VARCHAR(32) NOT NULL PRIMARY KEY,
            instance_name VARCHAR(128) NOT NULL,
            provider_id VARCHAR(32) NOT NULL,
            api_key VARCHAR(512) NOT NULL,
            status VARCHAR(32) DEFAULT 'active',
            extra VARCHAR(512) DEFAULT '{}',
            create_time BIGINT,
            create_date DATETIME,
            update_time BIGINT,
            update_date DATETIME,
            INDEX idx_provider_id (provider_id)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
        """
        self.db.execute_sql(create_sql)
        logger.info("Created tenant_model_instance table")


class TenantModelStage(MigrationStage):
    """Migrate tenant_llm to tenant_model"""

    name = "tenant_model"
    description = "Migrate tenant_llm to tenant_model (status='0' records, plus status='1' for empty-llm factories)"
    source_tables = ["tenant_llm", "tenant_model_provider", "tenant_model_instance"]
    target_tables = ["tenant_model"]

    @staticmethod
    def _get_empty_llm_factories() -> list[str]:
        """Load factory names whose llm field is an empty list from conf/llm_factories.json"""
        conf_path = os.path.join(PROJECT_BASE, "conf", "llm_factories.json")
        with open(conf_path, "r") as f:
            data = json.load(f)
        factories = []
        for key, items in data.items():
            if isinstance(items, list):
                for item in items:
                    if isinstance(item, dict):
                        llm = item.get("llm")
                        if isinstance(llm, list) and len(llm) == 0:
                            factories.append(item["name"])
        return factories

    def _build_status_condition(self) -> str:
        """Build SQL WHERE condition for status filtering"""
        empty_factories = self._get_empty_llm_factories()
        if empty_factories:
            placeholders = ", ".join(f"'{f}'" for f in empty_factories)
            return f"(tl.status = '0' OR (tl.status = '1' AND tl.llm_factory IN ({placeholders})))"
        return "tl.status = '0'"

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

        # Check if tenant_model_provider exists (dependency)
        if not self.db.table_exists("tenant_model_provider"):
            if self.dry_run:
                logger.info("[DRY RUN] Dependency table 'tenant_model_provider' does not exist. "
                           "Run 'tenant_model_provider' stage first or use --execute.")
                return False
            logger.warning("Dependency table 'tenant_model_provider' does not exist. "
                          "Please run 'tenant_model_provider' stage first.")
            return False

        # Check if tenant_model_instance exists (dependency)
        if not self.db.table_exists("tenant_model_instance"):
            if self.dry_run:
                logger.info("[DRY RUN] Dependency table 'tenant_model_instance' does not exist. "
                           "Run 'tenant_model_instance' stage first or use --execute.")
                return False
            logger.warning("Dependency table 'tenant_model_instance' does not exist. "
                          "Please run 'tenant_model_instance' stage first.")
            return False

        # Check if target table exists
        if not self.db.table_exists("tenant_model"):
            if self.dry_run:
                logger.info("[DRY RUN] Target table 'tenant_model' does not exist. "
                           "Use --execute to create and populate the table.")
                return False
            logger.info("Target table 'tenant_model' does not exist, will create")
            return True

        status_condition = self._build_status_condition()

        # Check if there's data to migrate
        cursor = self.db.execute_sql(
            f"SELECT COUNT(*) FROM ("
            f"  SELECT tl.id "
            f"  FROM tenant_llm tl "
            f"  INNER JOIN tenant_model_provider tmp ON tmp.tenant_id = tl.tenant_id AND tmp.provider_name = tl.llm_factory "
            f"  INNER JOIN tenant_model_instance tmi ON tmi.provider_id = tmp.id AND tmi.api_key = tl.api_key "
            f"  WHERE {status_condition} "
            f"  AND NOT EXISTS ("
            f"    SELECT 1 FROM tenant_model tm "
            f"    WHERE tm.provider_id = tmp.id AND tm.model_name = tl.llm_name AND tm.instance_id = tmi.id"
            f"  )"
            f") AS distinct_records"
        )
        count = cursor.fetchone()[0]

        if count == 0:
            self.mark_noop_completes_migration()
            logger.info("No new data to migrate from tenant_llm to tenant_model")
            return False

        logger.info(f"Found {count} rows to migrate from tenant_llm to tenant_model")
        return True

    def execute(self) -> tuple[int, list]:
        """Execute migration"""
        current_ts = self.current_timestamp()
        rows_inserted = 0

        # Check if tenant_model_provider exists (dependency)
        if not self.db.table_exists("tenant_model_provider"):
            logger.error("Dependency table 'tenant_model_provider' does not exist. "
                        "Please run 'tenant_model_provider' stage first.")
            return 0, []

        # Check if tenant_model_instance exists (dependency)
        if not self.db.table_exists("tenant_model_instance"):
            logger.error("Dependency table 'tenant_model_instance' does not exist. "
                        "Please run 'tenant_model_instance' stage first.")
            return 0, []

        # Check if target table exists
        if not self.db.table_exists("tenant_model"):
            if self.dry_run:
                logger.info("[DRY RUN] Target table 'tenant_model' does not exist. "
                           "Use --execute to create and populate the table.")
                return 0, []
            logger.info("Target table 'tenant_model' does not exist, will create")
            self.create_target_table()

        # If create_table_only mode, skip data migration
        if self.create_table_only:
            logger.info("[CREATE TABLE ONLY] Target table created/verified, skipping data migration")
            return 0, self.target_tables

        status_condition = self._build_status_condition()

        # Get records from tenant_llm with provider_id and instance_id lookup
        # Migrate status='0' records, plus status='1' for empty-llm factories
        cursor = self.db.execute_sql(
            f"SELECT tl.id, tl.llm_name, tmp.id as provider_id, tmi.id as instance_id, "
            f"       tl.model_type, tl.status "
            f"FROM tenant_llm tl "
            f"INNER JOIN tenant_model_provider tmp ON tmp.tenant_id = tl.tenant_id AND tmp.provider_name = tl.llm_factory "
            f"INNER JOIN tenant_model_instance tmi ON tmi.provider_id = tmp.id AND tmi.api_key = tl.api_key "
            f"WHERE {status_condition} "
            f"AND NOT EXISTS ("
            f"  SELECT 1 FROM tenant_model tm "
            f"  WHERE tm.provider_id = tmp.id AND tm.model_name = tl.llm_name AND tm.instance_id = tmi.id"
            f")"
        )

        records = cursor.fetchall()

        if not records:
            logger.info("No records to migrate")
            return 0, []

        logger.info(f"Migrating {len(records)} tenant_model records...")

        if self.dry_run:
            logger.info(f"[DRY RUN] Would insert {len(records)} records")
            for source_id, llm_name, provider_id, instance_id, model_type, status in records[:5]:
                logger.info(f"  model_name={llm_name}, provider_id={provider_id}, "
                           f"instance_id={instance_id}, model_type={model_type}")
            if len(records) > 5:
                logger.info(f"  ... and {len(records) - 5} more records")
            return len(records), self.target_tables

        # Insert records in batches
        batch_size = 100
        for i in range(0, len(records), batch_size):
            batch = records[i:i + batch_size]
            values = []
            for source_id, llm_name, provider_id, instance_id, model_type, status in batch:
                record_id = self.generate_uuid()
                model_name_escaped = llm_name.replace("'", "''") if llm_name else ""
                model_type_escaped = model_type.replace("'", "''") if model_type else ""
                status_val = "active" if status in ["1", "active", "enable"] else "inactive"
                values.append(f"('{record_id}', '{model_name_escaped}', '{provider_id}', "
                            f"'{instance_id}', '{model_type_escaped}', '{status_val}', "
                            f"{current_ts * 1000}, FROM_UNIXTIME({current_ts}), "
                            f"{current_ts * 1000}, FROM_UNIXTIME({current_ts}))")

            insert_sql = f"""
                INSERT INTO tenant_model 
                (id, model_name, provider_id, instance_id, model_type, status, 
                 create_time, create_date, update_time, update_date)
                VALUES {', '.join(values)}
            """
            self.db.execute_sql(insert_sql)
            rows_inserted += len(batch)
            logger.info(f"Inserted batch {i // batch_size + 1}: {len(batch)} records")

        return rows_inserted, self.target_tables

    def create_target_table(self):
        """Create tenant_model table"""
        create_sql = """
        CREATE TABLE IF NOT EXISTS tenant_model (
            id VARCHAR(32) NOT NULL PRIMARY KEY,
            model_name VARCHAR(128),
            provider_id VARCHAR(32) NOT NULL,
            instance_id VARCHAR(32) NOT NULL,
            model_type VARCHAR(32) NOT NULL,
            status VARCHAR(32) DEFAULT 'active',
            extra VARCHAR(1024) DEFAULT '{}',
            create_time BIGINT,
            create_date DATETIME,
            update_time BIGINT,
            update_date DATETIME,
            INDEX idx_instance_id (instance_id)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
        """
        self.db.execute_sql(create_sql)
        logger.info("Created tenant_model table")


class ModelIdConfigStage(MigrationStage):
    """Normalize stored model IDs from model@provider to model@default@provider."""

    name = "model_id_config"
    description = "Normalize stored model IDs in config columns to model@default@provider"
    source_tables = [
        "tenant",
        "knowledgebase",
        "document",
        "dialog",
        "memory",
        "search",
        "user_canvas",
        "canvas_template",
        "user_canvas_version",
        "api_4_conversation",
        "pipeline_operation_log",
        "connector",
        "evaluation_runs",
    ]
    target_tables = source_tables

    model_id_fields = {
        "llm_id",
        "embd_id",
        "embedding_model",
        "rerank_id",
        "asr_id",
        "img2txt_id",
        "tts_id",
        "ocr_id",
    }
    search_config_model_id_fields = {"chat_id"}
    scan_batch_size = 500
    string_columns = {
        "tenant": ("llm_id", "embd_id", "asr_id", "img2txt_id", "rerank_id", "tts_id", "ocr_id"),
        "knowledgebase": ("embd_id",),
        "dialog": ("llm_id", "rerank_id"),
        "memory": ("embd_id", "llm_id"),
    }
    json_columns = {
        "knowledgebase": ("parser_config",),
        "document": ("parser_config",),
        "search": ("search_config",),
        "user_canvas": ("dsl",),
        "canvas_template": ("dsl",),
        "user_canvas_version": ("dsl",),
        "api_4_conversation": ("dsl",),
        "pipeline_operation_log": ("dsl",),
        "connector": ("config",),
        "evaluation_runs": ("config_snapshot",),
    }

    def normalize_model_id(self, value):
        if not isinstance(value, str) or not value:
            return value, False

        parts = value.split("@")
        if len(parts) != 2:
            return value, False

        model_name, provider_name = parts
        if not model_name or not provider_name:
            return value, False

        return f"{model_name}@default@{provider_name}", True

    def normalize_config(self, value, path=None):
        path = path or ()

        if isinstance(value, dict):
            changed = False
            normalized = {}
            for key, item in value.items():
                key_path = path + (str(key),)
                should_normalize = key in self.model_id_fields or (
                    key in self.search_config_model_id_fields and "search_config" in path
                )
                if should_normalize:
                    normalized_item, item_changed = self.normalize_model_id(item)
                else:
                    normalized_item, item_changed = self.normalize_config(item, key_path)
                normalized[key] = normalized_item
                changed = changed or item_changed
            return normalized, changed

        if isinstance(value, list):
            changed = False
            normalized = []
            for index, item in enumerate(value):
                normalized_item, item_changed = self.normalize_config(item, path + (str(index),))
                normalized.append(normalized_item)
                changed = changed or item_changed
            return normalized, changed

        return value, False

    def existing_columns(self, table_columns):
        for table_name, columns in table_columns.items():
            if not self.db.table_exists(table_name):
                logger.info("Table '%s' does not exist, skipping", table_name)
                continue
            for column_name in columns:
                if not self.db.column_exists(table_name, column_name):
                    logger.info("Column '%s.%s' does not exist, skipping", table_name, column_name)
                    continue
                yield table_name, column_name

    def load_json_value(self, raw_value, table_name, column_name, row_id):
        if raw_value in (None, ""):
            return None, False
        if isinstance(raw_value, (dict, list)):
            return raw_value, True
        try:
            return json.loads(raw_value), True
        except (TypeError, json.JSONDecodeError):
            logger.warning(
                "Failed to parse JSON in %s.%s id=%s, skipping",
                table_name,
                column_name,
                row_id,
            )
            return None, False

    def iter_string_changes(self):
        for table_name, column_name in self.existing_columns(self.string_columns):
            cursor = self.db.execute_sql(
                f"SELECT id, `{column_name}` FROM `{table_name}` "
                f"WHERE `{column_name}` IS NOT NULL AND `{column_name}` != '' AND `{column_name}` LIKE %s",
                ("%@%",),
            )
            while True:
                rows = cursor.fetchmany(self.scan_batch_size)
                if not rows:
                    break
                for row_id, value in rows:
                    normalized, changed = self.normalize_model_id(value)
                    if changed:
                        yield table_name, column_name, row_id, normalized

    def iter_json_changes(self):
        for table_name, column_name in self.existing_columns(self.json_columns):
            cursor = self.db.execute_sql(
                f"SELECT id, `{column_name}` FROM `{table_name}` "
                f"WHERE `{column_name}` IS NOT NULL AND `{column_name}` != '' AND `{column_name}` LIKE %s",
                ("%@%",),
            )
            while True:
                rows = cursor.fetchmany(self.scan_batch_size)
                if not rows:
                    break
                for row_id, raw_value in rows:
                    config, loaded = self.load_json_value(raw_value, table_name, column_name, row_id)
                    if not loaded:
                        continue
                    normalized, changed = self.normalize_config(config, (column_name,))
                    if changed:
                        normalized_json = json.dumps(
                            normalized,
                            ensure_ascii=False,
                            separators=(",", ":"),
                        )
                        yield table_name, column_name, row_id, normalized_json

    def count_changes(self) -> tuple[int, set]:
        rows = 0
        tables = set()
        for table_name, _, _, _ in self.iter_string_changes():
            rows += 1
            tables.add(table_name)
        for table_name, _, _, _ in self.iter_json_changes():
            rows += 1
            tables.add(table_name)
        return rows, tables

    def check(self) -> bool:
        rows, tables = self.count_changes()
        if rows == 0:
            self.mark_noop_completes_migration()
            logger.info("No stored model IDs need normalization")
            return False
        logger.info(
            "Found %s rows to normalize across tables: %s",
            rows,
            ", ".join(sorted(tables)),
        )
        return True

    def execute(self) -> tuple[int, list]:
        if self.create_table_only:
            logger.info("[CREATE TABLE ONLY] No tables are created for this data migration")
            return 0, []

        rows_updated = 0
        tables_operated = set()

        for table_name, column_name, row_id, normalized in self.iter_string_changes():
            tables_operated.add(table_name)
            rows_updated += 1
            if rows_updated <= 10:
                logger.info(
                    "%s %s.%s id=%s -> %s",
                    "[DRY RUN] Would update" if self.dry_run else "Updating",
                    table_name,
                    column_name,
                    row_id,
                    normalized,
                )
            if not self.dry_run:
                self.db.execute_sql(
                    f"UPDATE `{table_name}` SET `{column_name}` = %s WHERE id = %s",
                    (normalized, row_id),
                )

        for table_name, column_name, row_id, normalized_json in self.iter_json_changes():
            tables_operated.add(table_name)
            rows_updated += 1
            if rows_updated <= 10:
                logger.info(
                    "%s %s.%s id=%s",
                    "[DRY RUN] Would update" if self.dry_run else "Updating",
                    table_name,
                    column_name,
                    row_id,
                )
            if not self.dry_run:
                self.db.execute_sql(
                    f"UPDATE `{table_name}` SET `{column_name}` = %s WHERE id = %s",
                    (normalized_json, row_id),
                )

        if rows_updated > 10:
            logger.info("... and %s more row updates", rows_updated - 10)

        if self.dry_run:
            logger.info("[DRY RUN] Would update %s rows", rows_updated)
        else:
            logger.info("Updated %s rows", rows_updated)

        return rows_updated, sorted(tables_operated)


# Registry of available migration stages
MIGRATION_STAGES = {
    'tenant_model_provider': TenantModelProviderStage,
    'tenant_model_instance': TenantModelInstanceStage,
    'tenant_model': TenantModelStage,
    'model_id_config': ModelIdConfigStage,
}


def list_available_stages():
    """List all available migration stages"""
    logger.info("Available migration stages:")
    for name, stage_cls in MIGRATION_STAGES.items():
        logger.info(f"  - {name}: {stage_cls.description}")
        logger.info(f"    Source tables: {stage_cls.source_tables}")
        logger.info(f"    Target tables: {stage_cls.target_tables}")


def run_migration(
    config: MigrationConfig,
    stages: list,
    dry_run: bool = True,
    create_table_only: bool = False,
    database_version: str | None = None,
    mark_database_version_on_success: bool = False,
):
    """Run migration with specified stages"""
    stats = MigrationStats()
    stats.start()
    
    db = MigrationDatabase(config)
    
    try:
        db.connect()

        if database_version:
            current_db_version = db.get_database_version()
            if should_skip_migration(current_db_version, database_version):
                logger.info(
                    "Database migration version is %s, target version is %s, skipping all stages",
                    current_db_version,
                    database_version,
                )
                return

            if current_db_version:
                logger.info(
                    "Current database migration version is %s, target version is %s",
                    current_db_version,
                    database_version,
                )
            else:
                logger.info(
                    "Database migration version marker is not set, target version is %s",
                    database_version,
                )
        
        total_stages = len(stages)
        all_stages_completed = True
        
        for idx, stage_name in enumerate(stages, 1):
            logger.info(f"{'=' * 60}")
            logger.info(f"Stage [{idx}/{total_stages}]: {stage_name}")
            logger.info(f"{'=' * 60}")
            
            if stage_name not in MIGRATION_STAGES:
                logger.error(f"Unknown stage: {stage_name}")
                stats.add_stage_stats(stage_name, [], 0, 0)
                all_stages_completed = False
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

        if (
            mark_database_version_on_success
            and not dry_run
            and not create_table_only
            and database_version
            and all_stages_completed
        ):
            db.set_database_version(database_version)
            logger.info("Marked database migration version as %s", database_version)

    finally:
        db.close()
        stats.end()
        stats.print_summary()


def check_database_version(config: MigrationConfig, target_version: str) -> int:
    db = MigrationDatabase(config)
    try:
        db.connect()
        current_db_version = db.get_database_version()
        if should_skip_migration(current_db_version, target_version):
            logger.info(
                "Database migration version is %s, target version is %s, migration is not needed",
                current_db_version,
                target_version,
            )
            return 0

        if current_db_version:
            logger.info(
                "Database migration version is %s, target version is %s, migration is needed",
                current_db_version,
                target_version,
            )
        else:
            logger.info(
                "Database migration version marker is not set, target version is %s, migration is needed",
                target_version,
            )
        return 1
    finally:
        db.close()


def mark_database_version(config: MigrationConfig, version: str) -> None:
    db = MigrationDatabase(config)
    try:
        db.connect()
        db.set_database_version(version)
        logger.info("Marked database migration version as %s", version)
    finally:
        db.close()


def main():
    parser = argparse.ArgumentParser(
        description='MySQL Data Migration Tool',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # List available stages
  python mysql_migration.py --list-stages

  # Check whether migration is needed for a target version
  python mysql_migration.py --check-database-version --database-version v0.26.0 --config /path/to/config.yaml

  # Mark database version separately
  python mysql_migration.py --mark-database-version --database-version v0.26.0 --config /path/to/config.yaml
  
  # Dry run (default - check only, no write) with config file
  python mysql_migration.py --stages tenant_model_provider --config /path/to/config.yaml
  
  # Dry run with command line MySQL connection
  python mysql_migration.py --stages tenant_model_provider --host localhost --port 3306 --user root --password secret
  
  # Create target tables only (no data migration)
  python mysql_migration.py --stages tenant_model_provider --config /path/to/config.yaml --create-table-only
  
  # Execute full migration (create tables and migrate data)
  python mysql_migration.py --stages tenant_model_provider --config /path/to/config.yaml --execute

  # Execute migration only when database version is lower than v0.26.0
  python mysql_migration.py --stages tenant_model_provider --config /path/to/config.yaml --execute --database-version v0.26.0

  # Execute migration and mark the database version when all stages succeed
  python mysql_migration.py --stages tenant_model_provider,tenant_model_instance,tenant_model,model_id_config --config /path/to/config.yaml --execute --database-version v0.26.0 --mark-database-version-on-success
  
  # Normalize legacy model IDs in stored configs
  python mysql_migration.py --stages model_id_config --config /path/to/config.yaml --execute

  # Run multiple stages
  python mysql_migration.py --stages stage1,stage2,stage3 --config /path/to/config.yaml --execute
"""
    )
    
    # MySQL connection options
    parser.add_argument('--host', type=str, default='localhost',
                       help='MySQL host (default: localhost)')
    parser.add_argument('--port', type=int, default=3306,
                       help='MySQL port (default: 3306)')
    parser.add_argument('--user', type=str, default='root',
                       help='MySQL user (default: root)')
    parser.add_argument('--password', type=str, default='',
                       help='MySQL password (default: empty)')
    parser.add_argument('--database', type=str, default='rag_flow',
                       help='MySQL database name (default: rag_flow)')
    
    # Configuration options
    parser.add_argument('--config', '-c', type=str, help='Path to YAML config file')
    
    # Migration options
    parser.add_argument('--stages', '-s', type=str, help='Comma-separated list of stages to run')
    parser.add_argument('--list-stages', '-l', action='store_true', help='List available stages')
    parser.add_argument('--check-database-version', action='store_true',
                       help='Check whether migration is needed for the target database version')
    parser.add_argument('--mark-database-version', action='store_true',
                       help='Write the database migration version marker and exit')
    parser.add_argument('--database-version', type=str, metavar='VERSION',
                       help='Database migration version used by check/mark commands and as the migration threshold for --stages')
    parser.add_argument('--mark-database-version-on-success', action='store_true',
                       help='When used with --stages and --execute, write --database-version after all stages succeed')
    parser.add_argument('--execute', '-e', action='store_true', default=False,
                       help='Execute full migration: create tables and migrate data')
    parser.add_argument('--create-table-only', action='store_true', default=False,
                       help='Only create target tables, skip data migration')
    
    args = parser.parse_args()
    
    # List stages and exit
    if args.list_stages:
        list_available_stages()
        return
    
    # Load configuration: command line args take precedence over config file
    if args.config:
        config = MigrationConfig.from_config_file(args.config)
        # Override with command line args if provided
        if args.host != 'localhost':
            config.host = args.host
        if args.port != 3306:
            config.port = args.port
        if args.user != 'root':
            config.user = args.user
        if args.password != '':
            config.password = args.password
        if args.database != 'rag_flow':
            config.database = args.database
    else:
        # Use command line args directly
        config = MigrationConfig(
            host=args.host,
            port=args.port,
            user=args.user,
            password=args.password,
            database=args.database
        )
    
    logger.info(f"MySQL Configuration: host={config.host}, port={config.port}, "
               f"user={config.user}, database={config.database}")

    if args.check_database_version and args.mark_database_version:
        logger.error("--check-database-version and --mark-database-version are mutually exclusive")
        sys.exit(1)

    if args.check_database_version:
        if not args.database_version:
            logger.error("--check-database-version requires --database-version")
            sys.exit(1)
        sys.exit(check_database_version(config, args.database_version))

    if args.mark_database_version:
        if not args.database_version:
            logger.error("--mark-database-version requires --database-version")
            sys.exit(1)
        mark_database_version(config, args.database_version)
        return

    if args.mark_database_version_on_success and not args.database_version:
        logger.error("--mark-database-version-on-success requires --database-version")
        sys.exit(1)
    
    # Three mutually exclusive modes: dry-run (default), create-table-only, execute
    if args.execute and args.create_table_only:
        logger.error("--execute and --create-table-only are mutually exclusive")
        sys.exit(1)

    if not args.stages:
        logger.error("No stages specified. Use --stages to specify stages or --list-stages to see available stages.")
        sys.exit(1)

    stages = [s.strip() for s in args.stages.split(',')]
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
        create_table_only=create_table_only,
        database_version=args.database_version,
        mark_database_version_on_success=args.mark_database_version_on_success,
    )


if __name__ == '__main__':
    main()
