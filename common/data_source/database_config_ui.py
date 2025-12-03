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
Database Connector UI Configuration

Provides UI schema and validation for database connector configuration.
This integrates with RAGFlow's data source configuration UI.
"""

from typing import Dict, List, Any, Optional, Tuple
from enum import Enum


class DatabaseUIFieldType(Enum):
    """UI field types"""
    TEXT = "text"
    PASSWORD = "password"
    NUMBER = "number"
    SELECT = "select"
    MULTI_SELECT = "multi_select"
    CHECKBOX = "checkbox"
    TEXTAREA = "textarea"
    JSON = "json"


class DatabaseUISchema:
    """UI schema for database connector configuration"""
    
    @staticmethod
    def get_mysql_schema() -> List[Dict[str, Any]]:
        """
        Get MySQL connector UI schema.
        
        Returns:
            List of field configurations
        """
        return [
            {
                "name": "db_type",
                "label": "Database Type",
                "type": DatabaseUIFieldType.SELECT.value,
                "required": True,
                "default": "mysql",
                "options": [
                    {"label": "MySQL", "value": "mysql"},
                    {"label": "MariaDB", "value": "mariadb"}
                ],
                "tooltip": "Select MySQL or MariaDB database type"
            },
            {
                "name": "host",
                "label": "Host",
                "type": DatabaseUIFieldType.TEXT.value,
                "required": True,
                "default": "localhost",
                "placeholder": "localhost or IP address",
                "tooltip": "Database server hostname or IP address"
            },
            {
                "name": "port",
                "label": "Port",
                "type": DatabaseUIFieldType.NUMBER.value,
                "required": True,
                "default": 3306,
                "min": 1,
                "max": 65535,
                "tooltip": "Database server port (default: 3306)"
            },
            {
                "name": "database",
                "label": "Database Name",
                "type": DatabaseUIFieldType.TEXT.value,
                "required": True,
                "placeholder": "my_database",
                "tooltip": "Name of the database to connect to"
            },
            {
                "name": "username",
                "label": "Username",
                "type": DatabaseUIFieldType.TEXT.value,
                "required": True,
                "placeholder": "db_user",
                "tooltip": "Database username"
            },
            {
                "name": "password",
                "label": "Password",
                "type": DatabaseUIFieldType.PASSWORD.value,
                "required": True,
                "placeholder": "••••••••",
                "tooltip": "Database password (will be encrypted)"
            },
            {
                "name": "sql_query",
                "label": "SQL Query",
                "type": DatabaseUIFieldType.TEXTAREA.value,
                "required": True,
                "placeholder": "SELECT * FROM products WHERE status = 'active'",
                "rows": 5,
                "tooltip": "SQL SELECT query to extract data. Use WHERE clauses to filter data."
            },
            {
                "name": "vectorization_fields",
                "label": "Vectorization Fields",
                "type": DatabaseUIFieldType.MULTI_SELECT.value,
                "required": True,
                "placeholder": "Select fields to vectorize",
                "tooltip": "Database columns to use for vector embeddings and search. These fields will be chunked and vectorized.",
                "dynamic_options": True,  # Populated after test connection
                "help_text": "Example: name, description, content"
            },
            {
                "name": "metadata_fields",
                "label": "Metadata Fields",
                "type": DatabaseUIFieldType.MULTI_SELECT.value,
                "required": False,
                "placeholder": "Select metadata fields",
                "tooltip": "Database columns to store as metadata. These won't be vectorized but will be searchable.",
                "dynamic_options": True,
                "help_text": "Example: id, category, created_at, price"
            },
            {
                "name": "primary_key_field",
                "label": "Primary Key Field",
                "type": DatabaseUIFieldType.TEXT.value,
                "required": False,
                "default": "id",
                "placeholder": "id",
                "tooltip": "Column name used as unique identifier for each row"
            },
            {
                "name": "sync_mode",
                "label": "Sync Mode",
                "type": DatabaseUIFieldType.SELECT.value,
                "required": True,
                "default": "batch",
                "options": [
                    {"label": "Batch (Full Sync)", "value": "batch"},
                    {"label": "Incremental (Timestamp-based)", "value": "incremental"}
                ],
                "tooltip": "Batch: sync all data. Incremental: sync only new/updated records based on timestamp."
            },
            {
                "name": "timestamp_field",
                "label": "Timestamp Field",
                "type": DatabaseUIFieldType.TEXT.value,
                "required": False,
                "placeholder": "updated_at",
                "tooltip": "Column name for timestamp-based incremental sync (required for incremental mode)",
                "conditional": {
                    "field": "sync_mode",
                    "value": "incremental"
                }
            },
            {
                "name": "batch_size",
                "label": "Batch Size",
                "type": DatabaseUIFieldType.NUMBER.value,
                "required": False,
                "default": 1000,
                "min": 100,
                "max": 10000,
                "tooltip": "Number of rows to process per batch (affects memory usage)"
            },
            {
                "name": "ssl_enabled",
                "label": "Enable SSL/TLS",
                "type": DatabaseUIFieldType.CHECKBOX.value,
                "required": False,
                "default": False,
                "tooltip": "Enable secure SSL/TLS connection to database"
            },
            {
                "name": "ssl_ca",
                "label": "SSL CA Certificate Path",
                "type": DatabaseUIFieldType.TEXT.value,
                "required": False,
                "placeholder": "/path/to/ca.pem",
                "tooltip": "Path to SSL Certificate Authority file",
                "conditional": {
                    "field": "ssl_enabled",
                    "value": True
                }
            }
        ]
    
    @staticmethod
    def get_postgresql_schema() -> List[Dict[str, Any]]:
        """
        Get PostgreSQL connector UI schema.
        
        Returns:
            List of field configurations
        """
        schema = DatabaseUISchema.get_mysql_schema()
        
        # Update database type options
        for field in schema:
            if field["name"] == "db_type":
                field["options"] = [
                    {"label": "PostgreSQL", "value": "postgresql"}
                ]
                field["default"] = "postgresql"
            
            # Update default port
            if field["name"] == "port":
                field["default"] = 5432
                field["tooltip"] = "Database server port (default: 5432)"
        
        return schema
    
    @staticmethod
    def get_advanced_options_schema() -> List[Dict[str, Any]]:
        """
        Get advanced configuration options schema.
        
        Returns:
            List of advanced field configurations
        """
        return [
            {
                "name": "pool_size",
                "label": "Connection Pool Size",
                "type": DatabaseUIFieldType.NUMBER.value,
                "required": False,
                "default": 5,
                "min": 1,
                "max": 20,
                "tooltip": "Number of database connections to maintain in pool",
                "category": "Performance"
            },
            {
                "name": "connection_timeout",
                "label": "Connection Timeout (seconds)",
                "type": DatabaseUIFieldType.NUMBER.value,
                "required": False,
                "default": 30,
                "min": 5,
                "max": 300,
                "tooltip": "Maximum time to wait for database connection",
                "category": "Performance"
            },
            {
                "name": "query_timeout",
                "label": "Query Timeout (seconds)",
                "type": DatabaseUIFieldType.NUMBER.value,
                "required": False,
                "default": 300,
                "min": 10,
                "max": 3600,
                "tooltip": "Maximum time to wait for query execution",
                "category": "Performance"
            },
            {
                "name": "enable_caching",
                "label": "Enable Query Caching",
                "type": DatabaseUIFieldType.CHECKBOX.value,
                "required": False,
                "default": True,
                "tooltip": "Cache query results to improve performance",
                "category": "Performance"
            },
            {
                "name": "cache_ttl",
                "label": "Cache TTL (seconds)",
                "type": DatabaseUIFieldType.NUMBER.value,
                "required": False,
                "default": 300,
                "min": 60,
                "max": 3600,
                "tooltip": "How long to cache query results",
                "category": "Performance",
                "conditional": {
                    "field": "enable_caching",
                    "value": True
                }
            },
            {
                "name": "enable_rate_limiting",
                "label": "Enable Rate Limiting",
                "type": DatabaseUIFieldType.CHECKBOX.value,
                "required": False,
                "default": True,
                "tooltip": "Limit query rate to prevent database overload",
                "category": "Performance"
            },
            {
                "name": "rate_limit_calls",
                "label": "Rate Limit (calls/minute)",
                "type": DatabaseUIFieldType.NUMBER.value,
                "required": False,
                "default": 100,
                "min": 10,
                "max": 1000,
                "tooltip": "Maximum queries per minute",
                "category": "Performance",
                "conditional": {
                    "field": "enable_rate_limiting",
                    "value": True
                }
            },
            {
                "name": "encrypt_credentials",
                "label": "Encrypt Credentials",
                "type": DatabaseUIFieldType.CHECKBOX.value,
                "required": False,
                "default": True,
                "tooltip": "Encrypt database credentials at rest",
                "category": "Security"
            }
        ]
    
    @staticmethod
    def validate_configuration(config: Dict[str, Any]) -> Tuple[bool, List[str]]:
        """
        Validate database configuration.
        
        Args:
            config: Configuration dictionary
            
        Returns:
            Tuple of (is_valid, error_messages)
        """
        errors = []
        
        # Required fields
        required_fields = [
            "db_type", "host", "port", "database",
            "username", "password", "sql_query", "vectorization_fields"
        ]
        
        for field in required_fields:
            if field not in config or not config[field]:
                errors.append(f"Required field missing: {field}")
        
        # Validate port
        if "port" in config:
            try:
                port = int(config["port"])
                if port < 1 or port > 65535:
                    errors.append("Port must be between 1 and 65535")
            except ValueError:
                errors.append("Port must be a number")
        
        # Validate SQL query
        if "sql_query" in config:
            query = config["sql_query"].strip().upper()
            if not query.startswith("SELECT"):
                errors.append("SQL query must be a SELECT statement")
            
            # Check for dangerous keywords
            dangerous_keywords = ["DROP", "DELETE", "TRUNCATE", "ALTER", "CREATE", "INSERT", "UPDATE"]
            for keyword in dangerous_keywords:
                if keyword in query:
                    errors.append(f"SQL query contains dangerous keyword: {keyword}")
        
        # Validate vectorization fields
        if "vectorization_fields" in config:
            if not isinstance(config["vectorization_fields"], list):
                errors.append("vectorization_fields must be a list")
            elif len(config["vectorization_fields"]) == 0:
                errors.append("At least one vectorization field required")
        
        # Validate incremental sync
        if config.get("sync_mode") == "incremental":
            if not config.get("timestamp_field"):
                errors.append("timestamp_field required for incremental sync mode")
        
        # Validate batch size
        if "batch_size" in config:
            try:
                batch_size = int(config["batch_size"])
                if batch_size < 100 or batch_size > 10000:
                    errors.append("batch_size must be between 100 and 10000")
            except ValueError:
                errors.append("batch_size must be a number")
        
        return (len(errors) == 0, errors)
    
    @staticmethod
    def get_example_configurations() -> Dict[str, Dict[str, Any]]:
        """
        Get example configurations for common use cases.
        
        Returns:
            Dictionary of example configurations
        """
        return {
            "product_catalog": {
                "name": "Product Catalog Sync",
                "description": "Sync product information from e-commerce database",
                "config": {
                    "db_type": "mysql",
                    "host": "localhost",
                    "port": 3306,
                    "database": "ecommerce",
                    "sql_query": "SELECT * FROM products WHERE status = 'active'",
                    "vectorization_fields": ["name", "description", "features"],
                    "metadata_fields": ["id", "category", "price", "sku", "created_at"],
                    "primary_key_field": "id",
                    "sync_mode": "incremental",
                    "timestamp_field": "updated_at",
                    "batch_size": 1000
                }
            },
            "customer_support": {
                "name": "Customer Support Tickets",
                "description": "Sync support tickets and knowledge base",
                "config": {
                    "db_type": "postgresql",
                    "host": "localhost",
                    "port": 5432,
                    "database": "support_db",
                    "sql_query": "SELECT * FROM tickets WHERE status IN ('resolved', 'closed')",
                    "vectorization_fields": ["title", "description", "resolution"],
                    "metadata_fields": ["ticket_id", "customer_id", "priority", "category", "resolved_at"],
                    "primary_key_field": "ticket_id",
                    "sync_mode": "incremental",
                    "timestamp_field": "resolved_at",
                    "batch_size": 500
                }
            },
            "documentation": {
                "name": "Internal Documentation",
                "description": "Sync internal documentation and wiki pages",
                "config": {
                    "db_type": "mysql",
                    "host": "localhost",
                    "port": 3306,
                    "database": "wiki_db",
                    "sql_query": "SELECT * FROM pages WHERE published = 1",
                    "vectorization_fields": ["title", "content", "summary"],
                    "metadata_fields": ["page_id", "author", "category", "tags", "last_modified"],
                    "primary_key_field": "page_id",
                    "sync_mode": "incremental",
                    "timestamp_field": "last_modified",
                    "batch_size": 100
                }
            },
            "faq_database": {
                "name": "FAQ Database",
                "description": "Sync frequently asked questions",
                "config": {
                    "db_type": "postgresql",
                    "host": "localhost",
                    "port": 5432,
                    "database": "faq_db",
                    "sql_query": "SELECT * FROM faqs WHERE active = true",
                    "vectorization_fields": ["question", "answer"],
                    "metadata_fields": ["faq_id", "category", "views", "helpful_count"],
                    "primary_key_field": "faq_id",
                    "sync_mode": "batch",
                    "batch_size": 500
                }
            }
        }


class DatabaseConnectionTester:
    """Test database connection and discover schema"""
    
    @staticmethod
    def test_connection(config: Dict[str, Any]) -> Dict[str, Any]:
        """
        Test database connection.
        
        Args:
            config: Database configuration
            
        Returns:
            Test result with status and details
        """
        result = {
            "success": False,
            "message": "",
            "connection_time_ms": 0,
            "server_version": None
        }
        
        try:
            import time
            from common.data_source.database_connector import create_mysql_connector, create_postgresql_connector
            
            start_time = time.time()
            
            # Create connector based on type
            if config["db_type"] in ["mysql", "mariadb"]:
                connector = create_mysql_connector(
                    host=config["host"],
                    port=config["port"],
                    database=config["database"],
                    username=config["username"],
                    password=config["password"],
                    sql_query="SELECT 1",
                    vectorization_fields=["dummy"]
                )
            else:
                connector = create_postgresql_connector(
                    host=config["host"],
                    port=config["port"],
                    database=config["database"],
                    username=config["username"],
                    password=config["password"],
                    sql_query="SELECT 1",
                    vectorization_fields=["dummy"]
                )
            
            # Test connection
            connector.validate_connector_settings()
            
            connection_time = (time.time() - start_time) * 1000
            
            result["success"] = True
            result["message"] = "Connection successful"
            result["connection_time_ms"] = round(connection_time, 2)
            
            # Get server version
            try:
                with connector.pool.get_connection() as conn:
                    cursor = conn.cursor()
                    if config["db_type"] in ["mysql", "mariadb"]:
                        cursor.execute("SELECT VERSION()")
                    else:
                        cursor.execute("SELECT version()")
                    version = cursor.fetchone()[0]
                    result["server_version"] = version
                    cursor.close()
            except:
                pass
            
            connector.close()
        
        except Exception as e:
            result["success"] = False
            result["message"] = str(e)
        
        return result
    
    @staticmethod
    def discover_schema(config: Dict[str, Any]) -> Dict[str, Any]:
        """
        Discover database schema from SQL query.
        
        Args:
            config: Database configuration
            
        Returns:
            Schema information with available fields
        """
        result = {
            "success": False,
            "fields": [],
            "sample_data": [],
            "row_count_estimate": 0
        }
        
        try:
            from common.data_source.database_connector import create_mysql_connector, create_postgresql_connector
            
            # Create connector
            if config["db_type"] in ["mysql", "mariadb"]:
                connector = create_mysql_connector(
                    host=config["host"],
                    port=config["port"],
                    database=config["database"],
                    username=config["username"],
                    password=config["password"],
                    sql_query=f"{config['sql_query']} LIMIT 10",
                    vectorization_fields=["dummy"]
                )
            else:
                connector = create_postgresql_connector(
                    host=config["host"],
                    port=config["port"],
                    database=config["database"],
                    username=config["username"],
                    password=config["password"],
                    sql_query=f"{config['sql_query']} LIMIT 10",
                    vectorization_fields=["dummy"]
                )
            
            connector.connect()
            
            # Execute query to get schema
            with connector.pool.get_connection() as conn:
                cursor = conn.cursor()
                cursor.execute(f"{config['sql_query']} LIMIT 10")
                
                # Get column information
                if config["db_type"] in ["mysql", "mariadb"]:
                    columns = cursor.description
                    fields = [
                        {
                            "name": col[0],
                            "type": str(col[1].__name__) if hasattr(col[1], '__name__') else "unknown",
                            "nullable": col[6] if len(col) > 6 else True
                        }
                        for col in columns
                    ]
                else:
                    columns = cursor.description
                    fields = [
                        {
                            "name": col.name,
                            "type": str(col.type_code) if hasattr(col, 'type_code') else "unknown",
                            "nullable": True
                        }
                        for col in columns
                    ]
                
                # Get sample data
                rows = cursor.fetchall()
                sample_data = [
                    {field["name"]: str(row[i])[:100] for i, field in enumerate(fields)}
                    for row in rows[:5]
                ]
                
                cursor.close()
            
            # Estimate row count
            try:
                with connector.pool.get_connection() as conn:
                    cursor = conn.cursor()
                    count_query = f"SELECT COUNT(*) FROM ({config['sql_query']}) AS subquery"
                    cursor.execute(count_query)
                    row_count = cursor.fetchone()[0]
                    result["row_count_estimate"] = row_count
                    cursor.close()
            except:
                pass
            
            result["success"] = True
            result["fields"] = fields
            result["sample_data"] = sample_data
            
            connector.close()
        
        except Exception as e:
            result["success"] = False
            result["error"] = str(e)
        
        return result


# Export UI schema for frontend
def get_ui_config() -> Dict[str, Any]:
    """
    Get complete UI configuration for database connector.
    
    Returns:
        UI configuration dictionary
    """
    return {
        "connector_type": "database",
        "display_name": "Database (MySQL/PostgreSQL)",
        "description": "Connect to relational databases for real-time data sync and vectorization",
        "icon": "database",
        "schemas": {
            "mysql": DatabaseUISchema.get_mysql_schema(),
            "postgresql": DatabaseUISchema.get_postgresql_schema(),
            "advanced": DatabaseUISchema.get_advanced_options_schema()
        },
        "examples": DatabaseUISchema.get_example_configurations(),
        "features": [
            "Real-time and batch synchronization",
            "Incremental sync with timestamp tracking",
            "Secure credential encryption",
            "Connection pooling for performance",
            "Query result caching",
            "SQL injection prevention",
            "Field-level transformations",
            "Metadata filtering support"
        ],
        "supported_databases": [
            {"name": "MySQL", "version": "5.7+"},
            {"name": "MariaDB", "version": "10.2+"},
            {"name": "PostgreSQL", "version": "10+"}
        ]
    }
