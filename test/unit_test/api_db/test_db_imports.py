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
Test backward compatibility of db_models.py shim.
Ensures all symbols can be imported from the legacy location.
"""


def test_import_base_models():
    """Test that base model classes can be imported from db_models"""
    from api.db.db_models import BaseModel, DataBaseModel

    assert BaseModel is not None
    assert DataBaseModel is not None


def test_import_connection_classes():
    """Test that connection classes can be imported from db_models"""
    from api.db.db_models import (
        DB,
        BaseDataBase,
        DatabaseLock,
        RetryingPooledMySQLDatabase,
        RetryingPooledPostgresqlDatabase,
        close_connection,
        with_retry,
    )

    assert DB is not None
    assert BaseDataBase is not None
    assert DatabaseLock is not None
    assert RetryingPooledMySQLDatabase is not None
    assert RetryingPooledPostgresqlDatabase is not None
    assert callable(close_connection)
    assert callable(with_retry)


def test_import_field_types():
    """Test that custom field types can be imported from db_models"""
    from api.db.db_models import (
        DateTimeTzField,
        JSONField,
        JsonSerializedField,
        ListField,
        LongTextField,
        SerializedField,
    )

    assert DateTimeTzField is not None
    assert JSONField is not None
    assert JsonSerializedField is not None
    assert ListField is not None
    assert LongTextField is not None
    assert SerializedField is not None


def test_import_field_helpers():
    """Test that field helper functions can be imported from db_models"""
    from api.db.db_models import (
        AUTO_DATE_TIMESTAMP_FIELD_PREFIX,
        CONTINUOUS_FIELD_TYPE,
        auto_date_timestamp_db_field,
        auto_date_timestamp_field,
        coerce_timestamp_range,
        is_continuous_field,
        remove_field_name_prefix,
    )

    assert AUTO_DATE_TIMESTAMP_FIELD_PREFIX is not None
    assert CONTINUOUS_FIELD_TYPE is not None
    assert callable(auto_date_timestamp_db_field)
    assert callable(auto_date_timestamp_field)
    assert callable(coerce_timestamp_range)
    assert callable(is_continuous_field)
    assert callable(remove_field_name_prefix)


def test_import_migrations():
    """Test that migration functions can be imported from db_models"""
    from api.db.db_models import (
        alter_db_add_column,
        alter_db_column_type,
        alter_db_rename_column,
        fill_db_model_object,
        init_database_tables,
        migrate_db,
    )

    assert callable(alter_db_add_column)
    assert callable(alter_db_column_type)
    assert callable(alter_db_rename_column)
    assert callable(fill_db_model_object)
    assert callable(init_database_tables)
    assert callable(migrate_db)


def test_import_model_classes():
    """Test that model classes can be imported from db_models"""
    from api.db.db_models import (
        APIToken,
        CanvasTemplate,
        Conversation,
        Dialog,
        Document,
        File2Document,
        Knowledgebase,
        LLM,
        Task,
        Tenant,
        User,
    )

    # Just verify they import successfully
    assert APIToken is not None
    assert CanvasTemplate is not None
    assert Conversation is not None
    assert Dialog is not None
    assert Document is not None
    assert File2Document is not None
    assert Knowledgebase is not None
    assert LLM is not None
    assert Task is not None
    assert Tenant is not None
    assert User is not None


def test_direct_module_imports():
    """Test that new modular structure can be imported directly"""
    # Test new structure imports work
    from api.db import base, connection, fields, migrations, models

    assert base is not None
    assert connection is not None
    assert fields is not None
    assert migrations is not None
    assert models is not None

    # Test models package exports
    from api.db.models import User, Tenant, Knowledgebase

    assert User is not None
    assert Tenant is not None
    assert Knowledgebase is not None


def test_db_connection_accessible():
    """Test that DB.connection is accessible and returns a database object"""
    from api.db.db_models import DB

    # DB.connection should be accessible and return a non-None value
    db1 = DB.connection
    db2 = DB.connection
    assert db1 is not None
    assert db2 is not None
