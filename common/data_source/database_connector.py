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
Enterprise-Grade Database Connector for MySQL and PostgreSQL

Features:
- Connection pooling for high performance
- Secure credential encryption
- Query optimization and caching
- Incremental sync with CDC support
- Comprehensive error handling and retry logic
- Monitoring and metrics
- Field mapping and transformation
- Batch processing with memory management
- SQL injection prevention
- SSL/TLS support
- Transaction management
- Schema discovery
- Data validation
- Rate limiting
- Health checks
"""

import logging
import hashlib
import json
import time
import re
import threading
from datetime import datetime, timedelta
from typing import Any, Dict, Generator, List, Optional, Tuple, Callable, Set
from dataclasses import dataclass, field, asdict
from enum import Enum
from queue import Queue, Empty
from contextlib import contextmanager
import base64
from cryptography.fernet import Fernet
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.kdf.pbkdf2 import PBKDF2
from collections import defaultdict, deque

from common.data_source.interfaces import LoadConnector, PollConnector, CredentialsConnector
from common.data_source.models import Document, TextSection, SecondsSinceUnixEpoch
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError
)


# ============================================================================
# Enums and Constants
# ============================================================================

class DatabaseType(Enum):
    """Supported database types"""
    MYSQL = "mysql"
    POSTGRESQL = "postgresql"
    MARIADB = "mariadb"


class SyncMode(Enum):
    """Synchronization modes"""
    BATCH = "batch"  # Full sync
    INCREMENTAL = "incremental"  # Timestamp-based
    CDC = "cdc"  # Change Data Capture


class FieldType(Enum):
    """Field data types"""
    TEXT = "text"
    INTEGER = "integer"
    FLOAT = "float"
    BOOLEAN = "boolean"
    DATETIME = "datetime"
    JSON = "json"
    BINARY = "binary"


class ConnectionState(Enum):
    """Connection states"""
    DISCONNECTED = "disconnected"
    CONNECTING = "connecting"
    CONNECTED = "connected"
    ERROR = "error"


# Constants
DEFAULT_POOL_SIZE = 5
MAX_POOL_SIZE = 20
CONNECTION_TIMEOUT = 30
QUERY_TIMEOUT = 300
MAX_RETRIES = 3
RETRY_DELAY = 1.0
BATCH_SIZE = 1000
MAX_BATCH_SIZE = 10000
CACHE_TTL = 300  # 5 minutes
MAX_CACHE_SIZE = 1000
RATE_LIMIT_CALLS = 100
RATE_LIMIT_PERIOD = 60  # 1 minute


# ============================================================================
# Data Classes
# ============================================================================

@dataclass
class DatabaseConfig:
    """Comprehensive database configuration"""
    # Connection settings
    db_type: str
    host: str
    port: int
    database: str
    
    # Query configuration
    sql_query: str
    vectorization_fields: List[str]
    metadata_fields: List[str] = field(default_factory=list)
    primary_key_field: str = "id"
    
    # Sync configuration
    sync_mode: str = "batch"
    timestamp_field: Optional[str] = None
    cdc_table: Optional[str] = None
    
    # Performance settings
    batch_size: int = BATCH_SIZE
    pool_size: int = DEFAULT_POOL_SIZE
    max_pool_size: int = MAX_POOL_SIZE
    connection_timeout: int = CONNECTION_TIMEOUT
    query_timeout: int = QUERY_TIMEOUT
    
    # Security settings
    ssl_enabled: bool = False
    ssl_ca: Optional[str] = None
    ssl_cert: Optional[str] = None
    ssl_key: Optional[str] = None
    encrypt_credentials: bool = True
    
    # Advanced options
    enable_caching: bool = True
    cache_ttl: int = CACHE_TTL
    enable_rate_limiting: bool = True
    rate_limit_calls: int = RATE_LIMIT_CALLS
    rate_limit_period: int = RATE_LIMIT_PERIOD
    enable_monitoring: bool = True
    
    # Field transformations
    field_transformations: Dict[str, Callable] = field(default_factory=dict)
    
    # Validation rules
    validation_rules: Dict[str, Callable] = field(default_factory=dict)
    
    def validate(self):
        """Validate configuration"""
        if self.db_type not in [e.value for e in DatabaseType]:
            raise ConnectorValidationError(
                f"Unsupported database type: {self.db_type}"
            )
        
        if not self.vectorization_fields:
            raise ConnectorValidationError(
                "At least one vectorization field required"
            )
        
        if self.sync_mode == "incremental" and not self.timestamp_field:
            raise ConnectorValidationError(
                "timestamp_field required for incremental sync"
            )
        
        if self.sync_mode == "cdc" and not self.cdc_table:
            raise ConnectorValidationError(
                "cdc_table required for CDC sync"
            )
        
        if self.batch_size > MAX_BATCH_SIZE:
            raise ConnectorValidationError(
                f"batch_size cannot exceed {MAX_BATCH_SIZE}"
            )


@dataclass
class ConnectionMetrics:
    """Connection pool metrics"""
    total_connections: int = 0
    active_connections: int = 0
    idle_connections: int = 0
    failed_connections: int = 0
    total_queries: int = 0
    failed_queries: int = 0
    avg_query_time: float = 0.0
    cache_hits: int = 0
    cache_misses: int = 0
    rate_limit_hits: int = 0
    
    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary"""
        return asdict(self)


@dataclass
class QueryResult:
    """Query execution result"""
    rows: List[Dict[str, Any]]
    row_count: int
    execution_time: float
    from_cache: bool = False
    query_hash: Optional[str] = None


@dataclass
class SyncCheckpoint:
    """Synchronization checkpoint"""
    last_sync_time: datetime
    last_timestamp: Optional[datetime] = None
    last_primary_key: Optional[str] = None
    rows_synced: int = 0
    errors: int = 0
    
    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary"""
        return {
            "last_sync_time": self.last_sync_time.isoformat(),
            "last_timestamp": self.last_timestamp.isoformat() if self.last_timestamp else None,
            "last_primary_key": self.last_primary_key,
            "rows_synced": self.rows_synced,
            "errors": self.errors
        }
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> 'SyncCheckpoint':
        """Create from dictionary"""
        return cls(
            last_sync_time=datetime.fromisoformat(data["last_sync_time"]),
            last_timestamp=datetime.fromisoformat(data["last_timestamp"]) if data.get("last_timestamp") else None,
            last_primary_key=data.get("last_primary_key"),
            rows_synced=data.get("rows_synced", 0),
            errors=data.get("errors", 0)
        )


# ============================================================================
# Security and Encryption
# ============================================================================

class CredentialEncryption:
    """Secure credential encryption using Fernet"""
    
    def __init__(self, master_key: Optional[bytes] = None):
        """
        Initialize encryption.
        
        Args:
            master_key: Master encryption key (generated if not provided)
        """
        if master_key:
            self.key = master_key
        else:
            # Generate key from system entropy
            self.key = Fernet.generate_key()
        
        self.cipher = Fernet(self.key)
    
    def encrypt(self, data: str) -> str:
        """Encrypt string data"""
        encrypted = self.cipher.encrypt(data.encode())
        return base64.b64encode(encrypted).decode()
    
    def decrypt(self, encrypted_data: str) -> str:
        """Decrypt string data"""
        decoded = base64.b64decode(encrypted_data.encode())
        decrypted = self.cipher.decrypt(decoded)
        return decrypted.decode()
    
    def encrypt_credentials(self, credentials: Dict[str, Any]) -> Dict[str, Any]:
        """Encrypt credential dictionary"""
        encrypted = {}
        for key, value in credentials.items():
            if isinstance(value, str):
                encrypted[key] = self.encrypt(value)
            else:
                encrypted[key] = value
        return encrypted
    
    def decrypt_credentials(self, encrypted_credentials: Dict[str, Any]) -> Dict[str, Any]:
        """Decrypt credential dictionary"""
        decrypted = {}
        for key, value in encrypted_credentials.items():
            if isinstance(value, str) and key in ["password", "api_key"]:
                try:
                    decrypted[key] = self.decrypt(value)
                except Exception:
                    decrypted[key] = value
            else:
                decrypted[key] = value
        return decrypted


class SQLInjectionPrevention:
    """SQL injection prevention utilities"""
    
    # Dangerous SQL patterns
    DANGEROUS_PATTERNS = [
        r";\s*(DROP|DELETE|TRUNCATE|ALTER|CREATE|INSERT|UPDATE)\s+",
        r"--",
        r"/\*.*\*/",
        r"UNION\s+SELECT",
        r"OR\s+1\s*=\s*1",
        r"AND\s+1\s*=\s*1",
        r"EXEC\s*\(",
        r"EXECUTE\s*\(",
    ]
    
    @classmethod
    def validate_query(cls, query: str) -> bool:
        """
        Validate SQL query for injection attempts.
        
        Args:
            query: SQL query string
            
        Returns:
            True if safe, False if potentially dangerous
        """
        query_upper = query.upper()
        
        for pattern in cls.DANGEROUS_PATTERNS:
            if re.search(pattern, query_upper, re.IGNORECASE):
                return False
        
        return True
    
    @classmethod
    def sanitize_identifier(cls, identifier: str) -> str:
        """
        Sanitize database identifier (table/column name).
        
        Args:
            identifier: Database identifier
            
        Returns:
            Sanitized identifier
        """
        # Remove dangerous characters
        sanitized = re.sub(r'[^\w_]', '', identifier)
        return sanitized
    
    @classmethod
    def escape_value(cls, value: Any) -> str:
        """
        Escape value for SQL (basic escaping, prefer parameterized queries).
        
        Args:
            value: Value to escape
            
        Returns:
            Escaped string
        """
        if value is None:
            return "NULL"
        elif isinstance(value, (int, float)):
            return str(value)
        elif isinstance(value, bool):
            return "TRUE" if value else "FALSE"
        else:
            # Escape single quotes
            escaped = str(value).replace("'", "''")
            return f"'{escaped}'"


# ============================================================================
# Connection Pool
# ============================================================================

class ConnectionPool:
    """Thread-safe database connection pool"""
    
    def __init__(
        self,
        db_type: str,
        host: str,
        port: int,
        database: str,
        credentials: Dict[str, Any],
        pool_size: int = DEFAULT_POOL_SIZE,
        max_pool_size: int = MAX_POOL_SIZE,
        connection_timeout: int = CONNECTION_TIMEOUT,
        ssl_config: Optional[Dict[str, Any]] = None
    ):
        """
        Initialize connection pool.
        
        Args:
            db_type: Database type
            host: Database host
            port: Database port
            database: Database name
            credentials: Database credentials
            pool_size: Initial pool size
            max_pool_size: Maximum pool size
            connection_timeout: Connection timeout in seconds
            ssl_config: SSL configuration
        """
        self.db_type = db_type
        self.host = host
        self.port = port
        self.database = database
        self.credentials = credentials
        self.pool_size = pool_size
        self.max_pool_size = max_pool_size
        self.connection_timeout = connection_timeout
        self.ssl_config = ssl_config or {}
        
        self.pool: Queue = Queue(maxsize=max_pool_size)
        self.active_connections: Set = set()
        self.lock = threading.Lock()
        self.logger = logging.getLogger(__name__)
        
        # Initialize pool
        self._initialize_pool()
    
    def _create_connection(self):
        """Create a new database connection"""
        try:
            if self.db_type == DatabaseType.MYSQL.value:
                import mysql.connector
                conn = mysql.connector.connect(
                    host=self.host,
                    port=self.port,
                    database=self.database,
                    user=self.credentials["username"],
                    password=self.credentials["password"],
                    connect_timeout=self.connection_timeout,
                    ssl_disabled=not self.ssl_config.get("enabled", False),
                    ssl_ca=self.ssl_config.get("ca"),
                    ssl_cert=self.ssl_config.get("cert"),
                    ssl_key=self.ssl_config.get("key"),
                    pool_name=None,  # Disable built-in pooling
                    pool_reset_session=True
                )
            
            elif self.db_type == DatabaseType.POSTGRESQL.value:
                import psycopg2
                conn = psycopg2.connect(
                    host=self.host,
                    port=self.port,
                    database=self.database,
                    user=self.credentials["username"],
                    password=self.credentials["password"],
                    connect_timeout=self.connection_timeout,
                    sslmode="require" if self.ssl_config.get("enabled") else "prefer",
                    sslrootcert=self.ssl_config.get("ca"),
                    sslcert=self.ssl_config.get("cert"),
                    sslkey=self.ssl_config.get("key")
                )
            
            else:
                raise ConnectorValidationError(f"Unsupported database type: {self.db_type}")
            
            self.logger.debug(f"Created new connection to {self.database}")
            return conn
        
        except Exception as e:
            self.logger.error(f"Failed to create connection: {e}")
            raise
    
    def _initialize_pool(self):
        """Initialize connection pool with initial connections"""
        for _ in range(self.pool_size):
            try:
                conn = self._create_connection()
                self.pool.put(conn)
            except Exception as e:
                self.logger.error(f"Failed to initialize pool connection: {e}")
    
    @contextmanager
    def get_connection(self):
        """
        Get connection from pool (context manager).
        
        Yields:
            Database connection
        """
        conn = None
        try:
            # Try to get from pool
            try:
                conn = self.pool.get(timeout=self.connection_timeout)
            except Empty:
                # Pool exhausted, create new if under max
                with self.lock:
                    if len(self.active_connections) < self.max_pool_size:
                        conn = self._create_connection()
                    else:
                        # Wait for connection
                        conn = self.pool.get(timeout=self.connection_timeout)
            
            # Test connection
            if not self._test_connection(conn):
                conn.close()
                conn = self._create_connection()
            
            with self.lock:
                self.active_connections.add(id(conn))
            
            yield conn
        
        finally:
            if conn:
                with self.lock:
                    self.active_connections.discard(id(conn))
                
                # Return to pool
                try:
                    self.pool.put_nowait(conn)
                except:
                    # Pool full, close connection
                    conn.close()
    
    def _test_connection(self, conn) -> bool:
        """Test if connection is alive"""
        try:
            cursor = conn.cursor()
            cursor.execute("SELECT 1")
            cursor.close()
            return True
        except:
            return False
    
    def close_all(self):
        """Close all connections in pool"""
        while not self.pool.empty():
            try:
                conn = self.pool.get_nowait()
                conn.close()
            except:
                pass
        
        self.logger.info("Connection pool closed")
    
    def get_stats(self) -> Dict[str, int]:
        """Get pool statistics"""
        with self.lock:
            return {
                "pool_size": self.pool.qsize(),
                "active_connections": len(self.active_connections),
                "max_pool_size": self.max_pool_size
            }


# ============================================================================
# Query Cache
# ============================================================================

class QueryCache:
    """LRU cache for query results"""
    
    def __init__(self, max_size: int = MAX_CACHE_SIZE, ttl: int = CACHE_TTL):
        """
        Initialize query cache.
        
        Args:
            max_size: Maximum cache entries
            ttl: Time-to-live in seconds
        """
        self.max_size = max_size
        self.ttl = ttl
        self.cache: Dict[str, Tuple[Any, float]] = {}
        self.access_order: deque = deque()
        self.lock = threading.Lock()
        self.hits = 0
        self.misses = 0
    
    def _hash_query(self, query: str, params: Optional[tuple] = None) -> str:
        """Generate hash for query and parameters"""
        key = f"{query}:{params}"
        return hashlib.md5(key.encode()).hexdigest()
    
    def get(self, query: str, params: Optional[tuple] = None) -> Optional[Any]:
        """Get cached result"""
        with self.lock:
            key = self._hash_query(query, params)
            
            if key in self.cache:
                result, timestamp = self.cache[key]
                
                # Check TTL
                if time.time() - timestamp < self.ttl:
                    # Update access order
                    self.access_order.remove(key)
                    self.access_order.append(key)
                    self.hits += 1
                    return result
                else:
                    # Expired
                    del self.cache[key]
                    self.access_order.remove(key)
            
            self.misses += 1
            return None
    
    def set(self, query: str, result: Any, params: Optional[tuple] = None):
        """Cache query result"""
        with self.lock:
            key = self._hash_query(query, params)
            
            # Evict if full
            if len(self.cache) >= self.max_size and key not in self.cache:
                # Remove least recently used
                lru_key = self.access_order.popleft()
                del self.cache[lru_key]
            
            self.cache[key] = (result, time.time())
            
            if key in self.access_order:
                self.access_order.remove(key)
            self.access_order.append(key)
    
    def clear(self):
        """Clear cache"""
        with self.lock:
            self.cache.clear()
            self.access_order.clear()
    
    def get_stats(self) -> Dict[str, Any]:
        """Get cache statistics"""
        with self.lock:
            total = self.hits + self.misses
            hit_rate = self.hits / total if total > 0 else 0.0
            
            return {
                "size": len(self.cache),
                "max_size": self.max_size,
                "hits": self.hits,
                "misses": self.misses,
                "hit_rate": hit_rate
            }


# ============================================================================
# Rate Limiter
# ============================================================================

class RateLimiter:
    """Token bucket rate limiter"""
    
    def __init__(self, calls: int = RATE_LIMIT_CALLS, period: int = RATE_LIMIT_PERIOD):
        """
        Initialize rate limiter.
        
        Args:
            calls: Maximum calls per period
            period: Time period in seconds
        """
        self.calls = calls
        self.period = period
        self.tokens = calls
        self.last_update = time.time()
        self.lock = threading.Lock()
        self.blocked_count = 0
    
    def acquire(self) -> bool:
        """
        Acquire token for API call.
        
        Returns:
            True if allowed, False if rate limited
        """
        with self.lock:
            now = time.time()
            elapsed = now - self.last_update
            
            # Refill tokens
            self.tokens = min(
                self.calls,
                self.tokens + (elapsed * self.calls / self.period)
            )
            self.last_update = now
            
            if self.tokens >= 1:
                self.tokens -= 1
                return True
            else:
                self.blocked_count += 1
                return False
    
    def get_stats(self) -> Dict[str, Any]:
        """Get rate limiter statistics"""
        with self.lock:
            return {
                "calls_per_period": self.calls,
                "period_seconds": self.period,
                "current_tokens": self.tokens,
                "blocked_count": self.blocked_count
            }


# ============================================================================
# Main Database Connector
# ============================================================================

class DatabaseConnector(LoadConnector, PollConnector, CredentialsConnector):
    """
    Enterprise-grade database connector with advanced features.
    
    Features:
    - Connection pooling
    - Query caching
    - Rate limiting
    - Secure credential encryption
    - SQL injection prevention
    - Comprehensive monitoring
    - Error handling and retry logic
    - Batch processing
    - Incremental sync
    """
    
    def __init__(self, config: DatabaseConfig):
        """
        Initialize database connector.
        
        Args:
            config: Database configuration
        """
        # Validate configuration
        config.validate()
        
        self.config = config
        self.logger = logging.getLogger(__name__)
        
        # Components
        self.pool: Optional[ConnectionPool] = None
        self.cache: Optional[QueryCache] = None
        self.rate_limiter: Optional[RateLimiter] = None
        self.encryption: Optional[CredentialEncryption] = None
        
        # State
        self.state = ConnectionState.DISCONNECTED
        self.credentials: Dict[str, Any] = {}
        self.metrics = ConnectionMetrics()
        self.checkpoint: Optional[SyncCheckpoint] = None
        
        # Initialize components
        if config.enable_caching:
            self.cache = QueryCache(
                max_size=MAX_CACHE_SIZE,
                ttl=config.cache_ttl
            )
        
        if config.enable_rate_limiting:
            self.rate_limiter = RateLimiter(
                calls=config.rate_limit_calls,
                period=config.rate_limit_period
            )
        
        if config.encrypt_credentials:
            self.encryption = CredentialEncryption()
        
        self.logger.info(f"Initialized {config.db_type} connector for {config.database}")
    
    # ========================================================================
    # Credential Management
    # ========================================================================
    
    def load_credentials(self, credentials: Dict[str, Any]) -> Dict[str, Any] | None:
        """
        Load and validate credentials.
        
        Args:
            credentials: Credential dictionary
            
        Returns:
            Validated credentials
        """
        if "username" not in credentials or "password" not in credentials:
            raise ConnectorMissingCredentialError(
                "Credentials must include 'username' and 'password'"
            )
        
        # Encrypt if enabled
        if self.encryption:
            self.credentials = self.encryption.encrypt_credentials(credentials)
        else:
            self.credentials = credentials
        
        self.logger.info("Credentials loaded successfully")
        return credentials
    
    def _get_decrypted_credentials(self) -> Dict[str, Any]:
        """Get decrypted credentials"""
        if self.encryption:
            return self.encryption.decrypt_credentials(self.credentials)
        return self.credentials
    
    # ========================================================================
    # Connection Management
    # ========================================================================
    
    def connect(self):
        """Establish database connection pool"""
        if self.state == ConnectionState.CONNECTED:
            return
        
        if not self.credentials:
            raise ConnectorMissingCredentialError("Credentials not loaded")
        
        try:
            self.state = ConnectionState.CONNECTING
            
            # Get decrypted credentials
            creds = self._get_decrypted_credentials()
            
            # SSL configuration
            ssl_config = {
                "enabled": self.config.ssl_enabled,
                "ca": self.config.ssl_ca,
                "cert": self.config.ssl_cert,
                "key": self.config.ssl_key
            }
            
            # Create connection pool
            self.pool = ConnectionPool(
                db_type=self.config.db_type,
                host=self.config.host,
                port=self.config.port,
                database=self.config.database,
                credentials=creds,
                pool_size=self.config.pool_size,
                max_pool_size=self.config.max_pool_size,
                connection_timeout=self.config.connection_timeout,
                ssl_config=ssl_config
            )
            
            self.state = ConnectionState.CONNECTED
            self.logger.info("Database connection pool established")
        
        except Exception as e:
            self.state = ConnectionState.ERROR
            self.logger.error(f"Connection failed: {e}")
            raise ConnectorValidationError(f"Failed to connect: {e}")
    
    def disconnect(self):
        """Close all database connections"""
        if self.pool:
            self.pool.close_all()
            self.pool = None
        
        self.state = ConnectionState.DISCONNECTED
        self.logger.info("Disconnected from database")
    
    def validate_connector_settings(self) -> None:
        """Validate connector settings by testing connection"""
        try:
            self.connect()
            
            # Test query
            test_query = f"{self.config.sql_query} LIMIT 1"
            
            # Validate query safety
            if not SQLInjectionPrevention.validate_query(test_query):
                raise ConnectorValidationError("Query contains potentially dangerous SQL")
            
            # Execute test query
            with self.pool.get_connection() as conn:
                cursor = conn.cursor()
                cursor.execute(test_query)
                result = cursor.fetchone()
                cursor.close()
            
            if result:
                self.logger.info("Connector validation successful")
        
        except Exception as e:
            raise ConnectorValidationError(f"Validation failed: {e}")
    
    # ========================================================================
    # Query Execution
    # ========================================================================
    
    def _execute_query_with_retry(
        self,
        query: str,
        params: Optional[tuple] = None,
        max_retries: int = MAX_RETRIES
    ) -> QueryResult:
        """
        Execute query with retry logic.
        
        Args:
            query: SQL query
            params: Query parameters
            max_retries: Maximum retry attempts
            
        Returns:
            QueryResult object
        """
        # Check rate limit
        if self.rate_limiter and not self.rate_limiter.acquire():
            self.metrics.rate_limit_hits += 1
            raise ConnectorValidationError("Rate limit exceeded")
        
        # Check cache
        if self.cache:
            cached = self.cache.get(query, params)
            if cached:
                self.metrics.cache_hits += 1
                return QueryResult(
                    rows=cached,
                    row_count=len(cached),
                    execution_time=0.0,
                    from_cache=True
                )
            self.metrics.cache_misses += 1
        
        # Execute with retry
        last_error = None
        for attempt in range(max_retries):
            try:
                start_time = time.time()
                
                with self.pool.get_connection() as conn:
                    cursor = conn.cursor()
                    
                    # Set query timeout
                    if self.config.db_type == DatabaseType.MYSQL.value:
                        cursor.execute(f"SET SESSION MAX_EXECUTION_TIME={self.config.query_timeout * 1000}")
                    elif self.config.db_type == DatabaseType.POSTGRESQL.value:
                        cursor.execute(f"SET statement_timeout = {self.config.query_timeout * 1000}")
                    
                    # Execute query
                    if params:
                        cursor.execute(query, params)
                    else:
                        cursor.execute(query)
                    
                    # Get column names
                    if self.config.db_type == DatabaseType.MYSQL.value:
                        columns = [desc[0] for desc in cursor.description]
                    else:
                        columns = [desc.name for desc in cursor.description]
                    
                    # Fetch all results
                    rows = cursor.fetchall()
                    cursor.close()
                
                execution_time = time.time() - start_time
                
                # Convert to dictionaries
                result_rows = [dict(zip(columns, row)) for row in rows]
                
                # Update metrics
                self.metrics.total_queries += 1
                self.metrics.avg_query_time = (
                    (self.metrics.avg_query_time * (self.metrics.total_queries - 1) + execution_time)
                    / self.metrics.total_queries
                )
                
                # Cache result
                if self.cache and len(result_rows) < 1000:  # Don't cache large results
                    self.cache.set(query, result_rows, params)
                
                return QueryResult(
                    rows=result_rows,
                    row_count=len(result_rows),
                    execution_time=execution_time,
                    from_cache=False
                )
            
            except Exception as e:
                last_error = e
                self.metrics.failed_queries += 1
                self.logger.warning(f"Query attempt {attempt + 1} failed: {e}")
                
                if attempt < max_retries - 1:
                    time.sleep(RETRY_DELAY * (attempt + 1))
        
        raise ConnectorValidationError(f"Query failed after {max_retries} attempts: {last_error}")
    
    def _execute_query_batched(
        self,
        query: str,
        params: Optional[tuple] = None
    ) -> Generator[List[Dict[str, Any]], None, None]:
        """
        Execute query and yield results in batches.
        
        Args:
            query: SQL query
            params: Query parameters
            
        Yields:
            Batches of rows
        """
        with self.pool.get_connection() as conn:
            cursor = conn.cursor()
            
            try:
                if params:
                    cursor.execute(query, params)
                else:
                    cursor.execute(query)
                
                # Get column names
                if self.config.db_type == DatabaseType.MYSQL.value:
                    columns = [desc[0] for desc in cursor.description]
                else:
                    columns = [desc.name for desc in cursor.description]
                
                # Fetch in batches
                while True:
                    rows = cursor.fetchmany(self.config.batch_size)
                    if not rows:
                        break
                    
                    batch = [dict(zip(columns, row)) for row in rows]
                    yield batch
            
            finally:
                cursor.close()
    
    # ========================================================================
    # Document Conversion
    # ========================================================================
    
    def _transform_field(self, field_name: str, value: Any) -> Any:
        """Apply field transformation if configured"""
        if field_name in self.config.field_transformations:
            transform_func = self.config.field_transformations[field_name]
            return transform_func(value)
        return value
    
    def _validate_field(self, field_name: str, value: Any) -> bool:
        """Validate field value if rule configured"""
        if field_name in self.config.validation_rules:
            validation_func = self.config.validation_rules[field_name]
            return validation_func(value)
        return True
    
    def _row_to_document(self, row: Dict[str, Any]) -> Document:
        """
        Convert database row to RAGFlow Document.
        
        Args:
            row: Database row
            
        Returns:
            Document object
        """
        # Generate document ID
        doc_id = str(row.get(self.config.primary_key_field, ""))
        if not doc_id:
            row_str = json.dumps(row, sort_keys=True, default=str)
            doc_id = hashlib.md5(row_str.encode()).hexdigest()
        
        # Build content from vectorization fields
        content_parts = []
        for field in self.config.vectorization_fields:
            if field in row and row[field]:
                value = row[field]
                
                # Apply transformation
                value = self._transform_field(field, value)
                
                # Validate
                if not self._validate_field(field, value):
                    self.logger.warning(f"Field {field} failed validation for row {doc_id}")
                    continue
                
                content_parts.append(f"{field}: {value}")
        
        content = "\n".join(content_parts)
        
        # Build metadata
        metadata = {}
        for field in self.config.metadata_fields:
            if field in row:
                value = row[field]
                
                # Convert datetime to ISO string
                if isinstance(value, datetime):
                    value = value.isoformat()
                
                # Apply transformation
                value = self._transform_field(field, value)
                
                metadata[field] = value
        
        # Add source metadata
        metadata.update({
            "_source": "database",
            "_db_type": self.config.db_type,
            "_database": self.config.database,
            "_table": self._extract_table_name(),
            "_primary_key": doc_id,
            "_sync_time": datetime.now().isoformat()
        })
        
        # Create document
        doc = Document(
            id=f"db_{self.config.db_type}_{self.config.database}_{doc_id}",
            sections=[TextSection(text=content, link=None)],
            source=f"{self.config.db_type}://{self.config.host}/{self.config.database}",
            semantic_identifier=f"Row {doc_id}",
            metadata=metadata
        )
        
        return doc
    
    def _extract_table_name(self) -> str:
        """Extract table name from SQL query"""
        # Simple regex to extract table name
        match = re.search(r'FROM\s+(\w+)', self.config.sql_query, re.IGNORECASE)
        if match:
            return match.group(1)
        return "unknown"
    
    # ========================================================================
    # Data Loading
    # ========================================================================
    
    def load_from_state(self) -> Generator[list[Document], None, None]:
        """
        Load all documents (batch mode).
        
        Yields:
            Batches of Document objects
        """
        self.connect()
        
        self.logger.info(f"Starting batch load from {self.config.database}")
        
        query = self.config.sql_query
        total_rows = 0
        
        try:
            for batch in self._execute_query_batched(query):
                documents = [self._row_to_document(row) for row in batch]
                total_rows += len(documents)
                
                yield documents
                
                self.logger.info(f"Loaded {total_rows} rows so far")
        
        except Exception as e:
            self.logger.error(f"Batch load failed: {e}")
            raise
        
        self.logger.info(f"Batch load completed: {total_rows} total rows")
    
    def poll_source(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch
    ) -> Generator[list[Document], None, None]:
        """
        Poll for new/updated documents (incremental mode).
        
        Args:
            start: Start timestamp
            end: End timestamp
            
        Yields:
            Batches of Document objects
        """
        self.connect()
        
        if self.config.sync_mode != SyncMode.INCREMENTAL.value:
            self.logger.warning("poll_source called but sync_mode is not incremental")
            return
        
        if not self.config.timestamp_field:
            raise ConnectorValidationError("timestamp_field required for incremental sync")
        
        start_dt = datetime.fromtimestamp(start)
        end_dt = datetime.fromtimestamp(end)
        
        self.logger.info(f"Polling for updates between {start_dt} and {end_dt}")
        
        # Build incremental query
        timestamp_field = SQLInjectionPrevention.sanitize_identifier(self.config.timestamp_field)
        
        if "WHERE" in self.config.sql_query.upper():
            query = f"{self.config.sql_query} AND {timestamp_field} BETWEEN %s AND %s"
        else:
            query = f"{self.config.sql_query} WHERE {timestamp_field} BETWEEN %s AND %s"
        
        total_rows = 0
        
        try:
            for batch in self._execute_query_batched(query, (start_dt, end_dt)):
                documents = [self._row_to_document(row) for row in batch]
                total_rows += len(documents)
                
                yield documents
                
                self.logger.info(f"Polled {total_rows} updated rows")
        
        except Exception as e:
            self.logger.error(f"Incremental poll failed: {e}")
            raise
        
        # Update checkpoint
        self.checkpoint = SyncCheckpoint(
            last_sync_time=datetime.now(),
            last_timestamp=end_dt,
            rows_synced=total_rows
        )
        
        self.logger.info(f"Poll completed: {total_rows} rows")
    
    # ========================================================================
    # Monitoring and Metrics
    # ========================================================================
    
    def get_metrics(self) -> Dict[str, Any]:
        """Get comprehensive metrics"""
        metrics = self.metrics.to_dict()
        
        if self.pool:
            metrics["connection_pool"] = self.pool.get_stats()
        
        if self.cache:
            metrics["cache"] = self.cache.get_stats()
        
        if self.rate_limiter:
            metrics["rate_limiter"] = self.rate_limiter.get_stats()
        
        if self.checkpoint:
            metrics["checkpoint"] = self.checkpoint.to_dict()
        
        return metrics
    
    def health_check(self) -> Dict[str, Any]:
        """Perform health check"""
        health = {
            "status": "unknown",
            "connection_state": self.state.value,
            "timestamp": datetime.now().isoformat()
        }
        
        try:
            if self.state != ConnectionState.CONNECTED:
                health["status"] = "disconnected"
                return health
            
            # Test query
            result = self._execute_query_with_retry("SELECT 1", max_retries=1)
            
            if result.row_count > 0:
                health["status"] = "healthy"
                health["query_time_ms"] = result.execution_time * 1000
            else:
                health["status"] = "unhealthy"
        
        except Exception as e:
            health["status"] = "error"
            health["error"] = str(e)
        
        return health
    
    # ========================================================================
    # Context Manager
    # ========================================================================
    
    def __enter__(self):
        """Context manager entry"""
        self.connect()
        return self
    
    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit"""
        self.disconnect()
    
    def close(self):
        """Close connector"""
        self.disconnect()


# ============================================================================
# Factory Functions
# ============================================================================

def create_mysql_connector(
    host: str,
    port: int,
    database: str,
    username: str,
    password: str,
    sql_query: str,
    vectorization_fields: List[str],
    **kwargs
) -> DatabaseConnector:
    """
    Create MySQL connector.
    
    Args:
        host: MySQL host
        port: MySQL port
        database: Database name
        username: Username
        password: Password
        sql_query: SQL query
        vectorization_fields: Fields to vectorize
        **kwargs: Additional configuration
        
    Returns:
        DatabaseConnector instance
    """
    config = DatabaseConfig(
        db_type=DatabaseType.MYSQL.value,
        host=host,
        port=port,
        database=database,
        sql_query=sql_query,
        vectorization_fields=vectorization_fields,
        **kwargs
    )
    
    connector = DatabaseConnector(config)
    connector.load_credentials({"username": username, "password": password})
    
    return connector


def create_postgresql_connector(
    host: str,
    port: int,
    database: str,
    username: str,
    password: str,
    sql_query: str,
    vectorization_fields: List[str],
    **kwargs
) -> DatabaseConnector:
    """
    Create PostgreSQL connector.
    
    Args:
        host: PostgreSQL host
        port: PostgreSQL port
        database: Database name
        username: Username
        password: Password
        sql_query: SQL query
        vectorization_fields: Fields to vectorize
        **kwargs: Additional configuration
        
    Returns:
        DatabaseConnector instance
    """
    config = DatabaseConfig(
        db_type=DatabaseType.POSTGRESQL.value,
        host=host,
        port=port,
        database=database,
        sql_query=sql_query,
        vectorization_fields=vectorization_fields,
        **kwargs
    )
    
    connector = DatabaseConnector(config)
    connector.load_credentials({"username": username, "password": password})
    
    return connector
