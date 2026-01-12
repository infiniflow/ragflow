#
# Base Peewee models and shared helpers
#
from __future__ import annotations

import logging
import operator
from typing import Optional

from peewee import BigIntegerField, CompositeKey, DateTimeField, Metadata, Model

from api.db.connection import DB
from api.db.fields import (
    AUTO_DATE_TIMESTAMP_FIELD_PREFIX,
    auto_date_timestamp_field,
    is_continuous_field,
    remove_field_name_prefix,
)
from common.time_utils import current_timestamp, date_string_to_timestamp, timestamp_to_date


class BaseModel(Model):
    _meta: Metadata
    create_time = BigIntegerField(null=True, index=True)
    create_date = DateTimeField(null=True, index=True)
    update_time = BigIntegerField(null=True, index=True)
    update_date = DateTimeField(null=True, index=True)

    def to_json(self):
        return self.to_dict()

    def to_dict(self):
        return self.__dict__["__data__"]

    def to_human_model_dict(self, only_primary_with: Optional[list[str]] = None):
        model_dict = self.__dict__["__data__"]

        if not only_primary_with:
            return {remove_field_name_prefix(k): v for k, v in model_dict.items()}

        human_model_dict = {}
        for k in self._meta.primary_key.field_names:
            if k in model_dict:
                human_model_dict[remove_field_name_prefix(k)] = model_dict[k]
        for k in only_primary_with:
            field_key = f"f_{k}"
            if field_key in model_dict:
                human_model_dict[k] = model_dict[field_key]
        return human_model_dict

    @property
    def meta(self) -> Metadata:
        return self._meta

    @classmethod
    def get_primary_keys_name(cls):
        return cls._meta.primary_key.field_names if isinstance(cls._meta.primary_key, CompositeKey) else [cls._meta.primary_key.name]

    @classmethod
    def getter_by(cls, attr):
        return operator.attrgetter(attr)(cls)

    @classmethod
    def query(cls, reverse=None, order_by=None, **kwargs):
        filters = []
        for f_n, f_v in kwargs.items():
            attr_name = f"{f_n}"
            if not hasattr(cls, attr_name) or f_v is None:
                continue
            if type(f_v) in {list, set}:
                f_v = list(f_v)
                if is_continuous_field(type(getattr(cls, attr_name))):
                    if len(f_v) == 2:
                        for i, v in enumerate(f_v):
                            if isinstance(v, str) and f_n in auto_date_timestamp_field():
                                f_v[i] = date_string_to_timestamp(v)
                        lt_value = f_v[0]
                        gt_value = f_v[1]
                        if lt_value is not None and gt_value is not None:
                            filters.append(cls.getter_by(attr_name).between(lt_value, gt_value))
                        elif lt_value is not None:
                            filters.append(operator.attrgetter(attr_name)(cls) >= lt_value)
                        elif gt_value is not None:
                            filters.append(operator.attrgetter(attr_name)(cls) <= gt_value)
                else:
                    filters.append(operator.attrgetter(attr_name)(cls) << f_v)
            else:
                filters.append(operator.attrgetter(attr_name)(cls) == f_v)
        if filters:
            query_records = cls.select().where(*filters)
            if reverse is not None:
                if not order_by or not hasattr(cls, f"{order_by}"):
                    order_by = "create_time"
                if reverse is True:
                    query_records = query_records.order_by(cls.getter_by(f"{order_by}").desc())
                elif reverse is False:
                    query_records = query_records.order_by(cls.getter_by(f"{order_by}").asc())
            return [query_record for query_record in query_records]
        return []

    @classmethod
    def insert(cls, __data=None, **insert):
        if isinstance(__data, dict) and __data:
            __data[cls._meta.combined["create_time"]] = current_timestamp()
        if insert:
            insert["create_time"] = current_timestamp()

        return super().insert(__data, **insert)

    @classmethod
    def _normalize_data(cls, data, kwargs):
        normalized = super()._normalize_data(data, kwargs)  # type: ignore[attr-defined]
        if not normalized:
            return {}

        if "update_time" in cls._meta.combined:
            normalized[cls._meta.combined["update_time"]] = current_timestamp()

        for f_n in AUTO_DATE_TIMESTAMP_FIELD_PREFIX:
            if {f"{f_n}_time", f"{f_n}_date"}.issubset(cls._meta.combined.keys()) and cls._meta.combined[f"{f_n}_time"] in normalized and normalized[cls._meta.combined[f"{f_n}_time"]] is not None:
                normalized[cls._meta.combined[f"{f_n}_date"]] = timestamp_to_date(normalized[cls._meta.combined[f"{f_n}_time"]])

        return normalized


class DataBaseModel(BaseModel):
    """
    Base model for all database models with compatibility validation.

    Extends BaseModel with database-specific compatibility checks.
    """

    class Meta:
        database = DB

    @classmethod
    def validate_fields(cls, db_type: Optional[str] = None):
        """
        Validate all fields are compatible with the target database.

        Args:
            db_type: Database type to validate against (defaults to settings.DATABASE_TYPE)

        This method checks each field in the model for compatibility with the
        target database and logs warnings or errors for incompatible fields.
        """
        # Import here to avoid circular dependency
        from api.db.migrations import DatabaseCompat
        from common import settings

        db_type = db_type or settings.DATABASE_TYPE
        db_type = db_type.lower()

        model_name = cls.__name__
        has_issues = False

        for field_name, field in cls._meta.fields.items():
            is_compatible, warning = DatabaseCompat.validate_field_for_db(field, db_type)

            if not is_compatible:
                logging.error(f"{model_name}.{field_name} is NOT compatible with {db_type}: {warning}")
                has_issues = True
            elif warning:
                logging.debug(f"{model_name}.{field_name} ({field.__class__.__name__}) - {db_type} note: {warning}")

        if not has_issues:
            logging.debug(f"{model_name} - all fields compatible with {db_type}")

        return not has_issues

    @classmethod
    def get_field_info(cls, field_name: str) -> dict:
        """
        Get detailed information about a specific field.

        Args:
            field_name: Name of the field to inspect

        Returns:
            dict: Field information including type, constraints, and compatibility
        """
        from api.db.migrations import DatabaseCompat
        from common import settings

        if field_name not in cls._meta.fields:
            return {"error": f"Field {field_name} not found in {cls.__name__}"}

        field = cls._meta.fields[field_name]
        db_type = settings.DATABASE_TYPE.lower()

        is_compatible, warning = DatabaseCompat.validate_field_for_db(field, db_type)

        info = {
            "field_name": field_name,
            "field_type": field.__class__.__name__,
            "db_field_type": field.field_type,
            "nullable": field.null,
            "indexed": field.index,
            "unique": field.unique,
            "default": field.default,
            "compatible_with_current_db": is_compatible,
            "compatibility_warning": warning,
        }

        # Add max_length for CharField/TextField
        if hasattr(field, "max_length"):
            info["max_length"] = field.max_length

        return info
