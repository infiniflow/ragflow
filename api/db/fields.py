#
# Field utilities extracted from db_models.py
#
from __future__ import annotations

import typing
from datetime import datetime, timezone

from peewee import CharField, DateTimeField, Field, FloatField, IntegerField, TextField

from api import utils
from api.db import SerializedType
from api.utils.configs import deserialize_b64, serialize_b64
from api.utils.json_encode import json_dumps, json_loads
from common import settings
from common.time_utils import date_string_to_timestamp

CONTINUOUS_FIELD_TYPE = {IntegerField, FloatField, DateTimeField}
AUTO_DATE_TIMESTAMP_FIELD_PREFIX = {"create", "start", "end", "update", "read_access", "write_access"}


class TextFieldTypeEnum:
    MYSQL = "LONGTEXT"
    POSTGRES = "TEXT"


class LongTextField(TextField):
    field_type = getattr(TextFieldTypeEnum, getattr(settings.DATABASE_TYPE, "upper", lambda: settings.DATABASE_TYPE)().upper(), "TEXT") if hasattr(settings, "DATABASE_TYPE") else "TEXT"


class JSONField(LongTextField):
    default_value = None

    def __init__(self, object_hook=None, object_pairs_hook=None, **kwargs):
        self._object_hook = object_hook
        self._object_pairs_hook = object_pairs_hook
        super().__init__(**kwargs)
        # Create instance-specific default value to avoid shared mutable state
        self.default_value = {}

    def db_value(self, value):
        if value is None:
            value = self.default_value
        return json_dumps(value)

    def python_value(self, value):
        if not value:
            return self.default_value
        return json_loads(value, object_hook=self._object_hook, object_pairs_hook=self._object_pairs_hook)


class ListField(JSONField):
    default_value = None

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        # Create instance-specific default value to avoid shared mutable state
        self.default_value = []


class SerializedField(LongTextField):
    def __init__(self, serialized_type=SerializedType.PICKLE, object_hook=None, object_pairs_hook=None, default_on_null=None, **kwargs):
        self._serialized_type = serialized_type
        self._object_hook = object_hook
        self._object_pairs_hook = object_pairs_hook
        self._default_on_null = default_on_null
        super().__init__(**kwargs)

    def db_value(self, value):
        if self._serialized_type == SerializedType.PICKLE:
            return serialize_b64(value, to_str=True)
        if self._serialized_type == SerializedType.JSON:
            if value is None:
                return None
            return json_dumps(value, with_type=True)
        raise ValueError(f"the serialized type {self._serialized_type} is not supported")

    def python_value(self, value):
        if self._serialized_type == SerializedType.PICKLE:
            return deserialize_b64(value)
        if self._serialized_type == SerializedType.JSON:
            if value is None:
                return self._default_on_null
            return json_loads(value, object_hook=self._object_hook, object_pairs_hook=self._object_pairs_hook)
        raise ValueError(f"the serialized type {self._serialized_type} is not supported")


class JsonSerializedField(SerializedField):
    def __init__(self, object_hook=utils.from_dict_hook, object_pairs_hook=None, **kwargs):
        super().__init__(serialized_type=SerializedType.JSON, object_hook=object_hook, object_pairs_hook=object_pairs_hook, **kwargs)


def is_continuous_field(cls: typing.Type) -> bool:
    if cls in CONTINUOUS_FIELD_TYPE:
        return True
    for parent in cls.__bases__:
        if parent in CONTINUOUS_FIELD_TYPE:
            return True
        if parent not in {Field, object} and is_continuous_field(parent):
            return True
    return False


def auto_date_timestamp_field():
    return {f"{f}_time" for f in AUTO_DATE_TIMESTAMP_FIELD_PREFIX}


def auto_date_timestamp_db_field():
    return {f"f_{f}_time" for f in AUTO_DATE_TIMESTAMP_FIELD_PREFIX}


def remove_field_name_prefix(field_name):
    return field_name[2:] if field_name.startswith("f_") else field_name


class DateTimeTzField(CharField):
    field_type = "VARCHAR"

    def db_value(self, value: datetime | None) -> str | None:
        if value is not None:
            if value.tzinfo is not None:
                return value.isoformat()
            return value.replace(tzinfo=timezone.utc).isoformat()
        return value

    def python_value(self, value: str | None) -> datetime | None:
        if value is None:
            return value
        dt = datetime.fromisoformat(value)
        if dt.tzinfo is None:
            return dt.replace(tzinfo=timezone.utc)
        return dt


def coerce_timestamp_range(field_name: str, values: list):
    # Converts date strings for auto date fields to timestamps in-place.
    for i, v in enumerate(values):
        if isinstance(v, str) and field_name in auto_date_timestamp_field():
            values[i] = date_string_to_timestamp(v)
    return values
