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
Unit tests for Database Connector
"""

import pytest
from unittest.mock import Mock, MagicMock, patch
from datetime import datetime

from common.data_source.database_connector import (
    DatabaseConnector,
    DatabaseConfig,
    create_mysql_connector,
    create_postgresql_connector
)
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError
)


class TestDatabaseConfig:
    """Test DatabaseConfig dataclass"""
    
    def test_default_config(self):
        """Test default configuration values"""
        config = DatabaseConfig(
            db_type="mysql",
            host="localhost",
            port=3306,
            database="test_db",
            username="user",
            password="pass",
            sql_query="SELECT * FROM products",
            vectorization_fields=["name", "description"],
            metadata_fields=["id", "category"]
        )
        
        assert config.db_type == "mysql"
        assert config.sync_mode == "batch"
        assert config.batch_size == 1000
        assert config.ssl_enabled is False


class TestDatabaseConnector:
    """Test DatabaseConnector class"""
    
    def test_initialization(self):
        """Test connector initialization"""
        connector = DatabaseConnector(
            db_type="mysql",
            host="localhost",
            port=3306,
            database="test_db",
            sql_query="SELECT * FROM products",
            vectorization_fields=["name", "description"],
            metadata_fields=["id", "price"]
        )
        
        assert connector.db_type == "mysql"
        assert connector.host == "localhost"
        assert connector.port == 3306
        assert connector.vectorization_fields == ["name", "description"]
        assert connector.metadata_fields == ["id", "price"]
    
    def test_invalid_db_type(self):
        """Test initialization with invalid database type"""
        with pytest.raises(ConnectorValidationError):
            DatabaseConnector(
                db_type="oracle",  # Not supported
                host="localhost",
                port=1521,
                database="test_db",
                sql_query="SELECT * FROM products",
                vectorization_fields=["name"]
            )
    
    def test_missing_vectorization_fields(self):
        """Test initialization without vectorization fields"""
        with pytest.raises(ConnectorValidationError):
            DatabaseConnector(
                db_type="mysql",
                host="localhost",
                port=3306,
                database="test_db",
                sql_query="SELECT * FROM products",
                vectorization_fields=[]  # Empty
            )
    
    def test_incremental_without_timestamp(self):
        """Test incremental mode without timestamp field"""
        with pytest.raises(ConnectorValidationError):
            DatabaseConnector(
                db_type="mysql",
                host="localhost",
                port=3306,
                database="test_db",
                sql_query="SELECT * FROM products",
                vectorization_fields=["name"],
                sync_mode="incremental",
                timestamp_field=None  # Missing
            )
    
    def test_load_credentials(self):
        """Test loading credentials"""
        connector = DatabaseConnector(
            db_type="mysql",
            host="localhost",
            port=3306,
            database="test_db",
            sql_query="SELECT * FROM products",
            vectorization_fields=["name"]
        )
        
        credentials = {
            "username": "test_user",
            "password": "test_pass"
        }
        
        result = connector.load_credentials(credentials)
        
        assert result == credentials
        assert connector.credentials == credentials
    
    def test_load_credentials_missing(self):
        """Test loading incomplete credentials"""
        connector = DatabaseConnector(
            db_type="mysql",
            host="localhost",
            port=3306,
            database="test_db",
            sql_query="SELECT * FROM products",
            vectorization_fields=["name"]
        )
        
        with pytest.raises(ConnectorMissingCredentialError):
            connector.load_credentials({"username": "test"})  # Missing password
    
    def test_row_to_document(self):
        """Test converting database row to document"""
        connector = DatabaseConnector(
            db_type="mysql",
            host="localhost",
            port=3306,
            database="test_db",
            sql_query="SELECT * FROM products",
            vectorization_fields=["name", "description"],
            metadata_fields=["id", "category"],
            primary_key_field="id"
        )
        
        row = {
            "id": 123,
            "name": "Test Product",
            "description": "A great product",
            "category": "Electronics",
            "price": 99.99
        }
        
        doc = connector._row_to_document(row)
        
        assert "Test Product" in doc.sections[0].text
        assert "A great product" in doc.sections[0].text
        assert doc.metadata["id"] == 123
        assert doc.metadata["category"] == "Electronics"
        assert doc.metadata["_source"] == "database"
        assert doc.metadata["_db_type"] == "mysql"
    
    def test_row_to_document_with_datetime(self):
        """Test converting row with datetime field"""
        connector = DatabaseConnector(
            db_type="postgresql",
            host="localhost",
            port=5432,
            database="test_db",
            sql_query="SELECT * FROM events",
            vectorization_fields=["title"],
            metadata_fields=["created_at"]
        )
        
        row = {
            "id": 1,
            "title": "Event Title",
            "created_at": datetime(2024, 1, 1, 12, 0, 0)
        }
        
        doc = connector._row_to_document(row)
        
        # Datetime should be converted to ISO format string
        assert isinstance(doc.metadata["created_at"], str)
        assert "2024-01-01" in doc.metadata["created_at"]
    
    def test_context_manager(self):
        """Test context manager usage"""
        connector = DatabaseConnector(
            db_type="mysql",
            host="localhost",
            port=3306,
            database="test_db",
            sql_query="SELECT * FROM products",
            vectorization_fields=["name"]
        )
        
        with connector as conn:
            assert conn is connector
        
        # Connection should be closed after context
        assert connector.connection is None


class TestFactoryFunctions:
    """Test factory functions"""
    
    def test_create_mysql_connector(self):
        """Test MySQL connector factory"""
        connector = create_mysql_connector(
            host="localhost",
            port=3306,
            database="test_db",
            username="user",
            password="pass",
            sql_query="SELECT * FROM products",
            vectorization_fields=["name", "description"]
        )
        
        assert connector.db_type == "mysql"
        assert connector.credentials["username"] == "user"
        assert connector.credentials["password"] == "pass"
    
    def test_create_postgresql_connector(self):
        """Test PostgreSQL connector factory"""
        connector = create_postgresql_connector(
            host="localhost",
            port=5432,
            database="test_db",
            username="user",
            password="pass",
            sql_query="SELECT * FROM products",
            vectorization_fields=["name", "description"]
        )
        
        assert connector.db_type == "postgresql"
        assert connector.credentials["username"] == "user"
    
    def test_factory_with_optional_params(self):
        """Test factory with optional parameters"""
        connector = create_mysql_connector(
            host="localhost",
            port=3306,
            database="test_db",
            username="user",
            password="pass",
            sql_query="SELECT * FROM products",
            vectorization_fields=["name"],
            metadata_fields=["id", "category"],
            sync_mode="incremental",
            timestamp_field="updated_at",
            batch_size=500,
            ssl_enabled=True
        )
        
        assert connector.metadata_fields == ["id", "category"]
        assert connector.sync_mode == "incremental"
        assert connector.timestamp_field == "updated_at"
        assert connector.batch_size == 500
        assert connector.ssl_enabled is True


class TestDocumentConversion:
    """Test document conversion logic"""
    
    def test_multiple_vectorization_fields(self):
        """Test combining multiple fields for vectorization"""
        connector = DatabaseConnector(
            db_type="mysql",
            host="localhost",
            port=3306,
            database="test_db",
            sql_query="SELECT * FROM products",
            vectorization_fields=["name", "description", "features"]
        )
        
        row = {
            "id": 1,
            "name": "Product A",
            "description": "Description A",
            "features": "Feature 1, Feature 2"
        }
        
        doc = connector._row_to_document(row)
        content = doc.sections[0].text
        
        assert "Product A" in content
        assert "Description A" in content
        assert "Feature 1" in content
    
    def test_missing_vectorization_field(self):
        """Test handling missing vectorization field"""
        connector = DatabaseConnector(
            db_type="mysql",
            host="localhost",
            port=3306,
            database="test_db",
            sql_query="SELECT * FROM products",
            vectorization_fields=["name", "description"]
        )
        
        row = {
            "id": 1,
            "name": "Product A"
            # description is missing
        }
        
        doc = connector._row_to_document(row)
        
        # Should not crash, just skip missing field
        assert "Product A" in doc.sections[0].text
    
    def test_document_id_generation(self):
        """Test document ID generation"""
        connector = DatabaseConnector(
            db_type="mysql",
            host="localhost",
            port=3306,
            database="test_db",
            sql_query="SELECT * FROM products",
            vectorization_fields=["name"],
            primary_key_field="product_id"
        )
        
        row = {
            "product_id": "ABC123",
            "name": "Product"
        }
        
        doc = connector._row_to_document(row)
        
        assert "ABC123" in doc.id
        assert doc.metadata["_primary_key"] == "ABC123"


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
