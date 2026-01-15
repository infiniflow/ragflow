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
Unit tests for custom database field types and helpers.
"""

from api.db.fields import (
    AUTO_DATE_TIMESTAMP_FIELD_PREFIX,
    CONTINUOUS_FIELD_TYPE,
    DateTimeTzField,
    JSONField,
    ListField,
    LongTextField,
    SerializedField,
    auto_date_timestamp_field,
    remove_field_name_prefix,
)


def test_auto_date_timestamp_field_prefix():
    """Test AUTO_DATE_TIMESTAMP_FIELD_PREFIX is a set with expected values"""
    assert isinstance(AUTO_DATE_TIMESTAMP_FIELD_PREFIX, set)
    assert "create" in AUTO_DATE_TIMESTAMP_FIELD_PREFIX
    assert "update" in AUTO_DATE_TIMESTAMP_FIELD_PREFIX


def test_continuous_field_type():
    """Test CONTINUOUS_FIELD_TYPE is a set with Peewee field types"""
    assert isinstance(CONTINUOUS_FIELD_TYPE, set)
    from peewee import IntegerField, FloatField, DateTimeField
    assert IntegerField in CONTINUOUS_FIELD_TYPE
    assert FloatField in CONTINUOUS_FIELD_TYPE
    assert DateTimeField in CONTINUOUS_FIELD_TYPE


def test_auto_date_timestamp_field():
    """Test auto_date_timestamp_field returns set of field names"""
    result = auto_date_timestamp_field()
    assert isinstance(result, set)
    assert "create_time" in result
    assert "update_time" in result


def test_remove_field_name_prefix():
    """Test remove_field_name_prefix removes f_ prefix"""
    assert remove_field_name_prefix("f_user_id") == "user_id"
    assert remove_field_name_prefix("user_id") == "user_id"


def test_json_field_creation():
    """Test JSONField can be instantiated"""
    field = JSONField()
    assert field is not None
    assert hasattr(field, "db_value")
    assert hasattr(field, "python_value")


def test_list_field_creation():
    """Test ListField can be instantiated"""
    field = ListField()
    assert field is not None
    assert hasattr(field, "default_value")


def test_serialized_field_creation():
    """Test SerializedField can be instantiated"""
    field = SerializedField()
    assert field is not None
    assert hasattr(field, "db_value")
    assert hasattr(field, "python_value")


def test_datetime_tz_field_creation():
    """Test DateTimeTzField can be instantiated"""
    field = DateTimeTzField()
    assert field is not None
    assert hasattr(field, "db_value")
    assert hasattr(field, "python_value")


def test_long_text_field_creation():
    """Test LongTextField can be instantiated"""
    field = LongTextField()
    assert field is not None


def test_field_imports_from_legacy():
    """Ensure fields can be imported from legacy db_models location"""
    from api.db.db_models import (
        DateTimeTzField as LegacyDateTimeTzField,
        JSONField as LegacyJSONField,
        ListField as LegacyListField,
        LongTextField as LegacyLongTextField,
        SerializedField as LegacySerializedField,
    )

    # Verify they're the same classes
    assert LegacyDateTimeTzField is DateTimeTzField
    assert LegacyJSONField is JSONField
    assert LegacyListField is ListField
    assert LegacyLongTextField is LongTextField
    assert LegacySerializedField is SerializedField
