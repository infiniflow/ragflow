#
# Database compatibility matrix and multi-database support utilities
#
# Centralizes database-specific logic and validates field compatibility
# across MySQL and PostgreSQL. Provides a foundation for future multi-database
# support and helps identify potential compatibility issues early.
#
from __future__ import annotations

import logging

from common import settings


class DatabaseCompat:
    """
    Database capability matrix and compatibility checks.

    Centralizes database-specific logic and validates field compatibility
    across MySQL and PostgreSQL. Provides a foundation for future multi-database
    support and helps identify potential compatibility issues early.
    """

    # Capability matrix: defines what each database supports
    CAPABILITIES = {
        "mysql": {
            "full_text_search": True,  # MySQL has FULLTEXT indexes
            "json_functions": True,  # JSON_EXTRACT, JSON_SET, etc.
            "auto_increment": True,  # AUTO_INCREMENT columns
            "sequence_support": False,  # No SEQUENCE objects (uses AUTO_INCREMENT)
            "type_casting": "limited",  # Type casting is more restrictive
            "max_varchar": 65535,  # Maximum VARCHAR length
            "case_sensitive_collation": False,  # Default collation is case-insensitive
            "array_support": False,  # No native array type
            "jsonb_support": False,  # Only JSON (text-based)
            "enum_support": True,  # Native ENUM type
            "transaction_ddl": False,  # DDL not transactional
        },
        "postgres": {
            "full_text_search": True,  # TSVECTOR and text search
            "json_functions": True,  # JSONB operators and functions
            "auto_increment": False,  # Uses SERIAL/SEQUENCES instead
            "sequence_support": True,  # Native SEQUENCE objects
            "type_casting": "full",  # Rich type casting support
            "max_varchar": None,  # No practical limit (uses TEXT internally)
            "case_sensitive_collation": True,  # Default collation is case-sensitive
            "array_support": True,  # Native array types
            "jsonb_support": True,  # Binary JSON format
            "enum_support": True,  # Native ENUM type (via CREATE TYPE)
            "transaction_ddl": True,  # DDL is transactional
        },
    }

    # Field type equivalence map: MySQL -> PostgreSQL
    TYPE_EQUIVALENTS = {
        "mysql_to_postgres": {
            "LONGTEXT": "TEXT",
            "MEDIUMTEXT": "TEXT",
            "TINYTEXT": "TEXT",
            "VARCHAR": "VARCHAR",
            "INT": "INTEGER",
            "BIGINT": "BIGINT",
            "FLOAT": "REAL",
            "DOUBLE": "DOUBLE PRECISION",
            "DATETIME": "TIMESTAMP",
            "TIMESTAMP": "TIMESTAMP WITH TIME ZONE",
            "JSON": "JSONB",  # Prefer JSONB for performance
            "ENUM": "VARCHAR",  # PostgreSQL ENUM requires CREATE TYPE
        },
        "postgres_to_mysql": {
            "TEXT": "LONGTEXT",
            "VARCHAR": "VARCHAR",
            "INTEGER": "INT",
            "BIGINT": "BIGINT",
            "REAL": "FLOAT",
            "DOUBLE PRECISION": "DOUBLE",
            "TIMESTAMP": "DATETIME",
            "TIMESTAMP WITH TIME ZONE": "TIMESTAMP",
            "JSONB": "JSON",
            "ARRAY": None,  # No MySQL equivalent
        },
    }

    # Field compatibility warnings
    FIELD_WARNINGS = {
        "mysql": {
            "LongTextField": None,  # Native support
            "JSONField": "JSON is stored as text in MySQL, use JSONB-like operations carefully",
            "DateTimeTzField": "Timezone handling may differ from PostgreSQL",
            "SerializedField": None,  # Works via custom encoding
        },
        "postgres": {
            "LongTextField": None,  # Native TEXT support
            "JSONField": "Consider using JSONB field type for better performance",
            "DateTimeTzField": None,  # Native timezone support
            "SerializedField": None,  # Works via custom encoding
        },
    }

    @staticmethod
    def is_capable(db_type: str, capability: str) -> bool:
        """
        Check if a database supports a specific capability.

        Args:
            db_type: Database type (mysql or postgres)
            capability: Capability name from CAPABILITIES dict

        Returns:
            bool: True if supported, False otherwise

        Example:
            >>> DatabaseCompat.is_capable("postgres", "jsonb_support")
            True
            >>> DatabaseCompat.is_capable("mysql", "jsonb_support")
            False
        """
        db_type = db_type.lower()
        if db_type not in DatabaseCompat.CAPABILITIES:
            logging.warning(f"Unknown database type: {db_type}")
            return False

        return bool(DatabaseCompat.CAPABILITIES[db_type].get(capability, False))

    @staticmethod
    def get_capability_value(db_type: str, capability: str):
        """
        Get the raw capability value for a database.

        Args:
            db_type: Database type (mysql or postgres)
            capability: Capability name from CAPABILITIES dict

        Returns:
            The raw capability value or None

        Example:
            >>> DatabaseCompat.get_capability_value("postgres", "jsonb_support")
            True
        """
        db_type = db_type.lower()
        if db_type not in DatabaseCompat.CAPABILITIES:
            logging.warning(f"Unknown database type: {db_type}")
            return None

        return DatabaseCompat.CAPABILITIES[db_type].get(capability)

    @staticmethod
    def get_equivalent_type(field_type: str, source_db: str, target_db: str) -> str | None:
        """
        Get equivalent field type for target database.

        Args:
            field_type: Source field type
            source_db: Source database type (mysql or postgres)
            target_db: Target database type (mysql or postgres)

        Returns:
            str | None: Equivalent type in target database, or None if no equivalent

        Example:
            >>> DatabaseCompat.get_equivalent_type("LONGTEXT", "mysql", "postgres")
            'TEXT'
            >>> DatabaseCompat.get_equivalent_type("ARRAY", "postgres", "mysql")
            None
        """
        source_db = source_db.lower()
        target_db = target_db.lower()

        if source_db == target_db:
            return field_type  # No conversion needed

        direction = f"{source_db}_to_{target_db}"
        type_map = DatabaseCompat.TYPE_EQUIVALENTS.get(direction, {})

        equivalent = type_map.get(field_type.upper())
        if equivalent is None and field_type.upper() in type_map:
            logging.warning(f"No equivalent type in {target_db} for {source_db} type: {field_type}")

        return equivalent

    @staticmethod
    def validate_field_for_db(field, db_type: str) -> tuple[bool, str | None]:
        """
        Validate that a field is compatible with the target database.

        Args:
            field: Peewee field instance
            db_type: Database type (mysql or postgres)

        Returns:
            tuple: (is_compatible: bool, warning: str | None)

        Example:
            >>> from api.db.fields import JSONField
            >>> field = JSONField()
            >>> is_compat, warning = DatabaseCompat.validate_field_for_db(field, "mysql")
            >>> print(is_compat)
            True
        """
        db_type = db_type.lower()
        field_class_name = field.__class__.__name__

        # Check if there's a specific warning for this field type
        warnings_for_db = DatabaseCompat.FIELD_WARNINGS.get(db_type, {})
        warning = warnings_for_db.get(field_class_name)

        # All current field types are compatible, but may have warnings
        is_compatible = True

        # Validate that db_type is recognized
        db_capabilities = DatabaseCompat.CAPABILITIES.get(db_type, {})
        if not db_capabilities:
            logging.warning(f"Unknown database type: {db_type}")
            return (is_compatible, warning)

        # Special checks for specific scenarios
        if field_class_name == "JSONField" and db_type == "mysql":
            # MySQL JSON is text-based, less efficient than PostgreSQL JSONB
            # MySQL stores JSON as a string, which may impact query performance
            if not warning:
                warning = DatabaseCompat.FIELD_WARNINGS.get("mysql", {}).get("JSONField")

        if hasattr(field, "max_length"):
            max_len = field.max_length
            max_varchar = db_capabilities.get("max_varchar")
            # Only validate max_varchar if it's defined (not None) and field has a max_length
            if max_varchar is not None and max_len and max_len > max_varchar:
                warning = f"VARCHAR({max_len}) exceeds {db_type} maximum of {max_varchar}"
                is_compatible = False

        return (is_compatible, warning)

    @staticmethod
    def log_compatibility_info(model_class, db_type: str):
        """
        Log compatibility information for all fields in a model.

        Args:
            model_class: Peewee model class
            db_type: Database type to validate against
        """
        db_type = db_type.lower()
        model_name = model_class.__name__

        for field_name, field in model_class._meta.fields.items():
            is_compatible, warning = DatabaseCompat.validate_field_for_db(field, db_type)

            if not is_compatible:
                logging.error(f"{model_name}.{field_name} is NOT compatible with {db_type}: {warning}")
            elif warning:
                logging.debug(f"{model_name}.{field_name} compatibility note for {db_type}: {warning}")

    @staticmethod
    def get_capabilities(db_type: str) -> dict:
        """
        Get all capabilities for a database type.

        Args:
            db_type: Database type (mysql or postgres)

        Returns:
            dict: Capability dictionary, or empty dict if unknown type
        """
        db_type = db_type.lower()
        return DatabaseCompat.CAPABILITIES.get(db_type, {})

    @staticmethod
    def requires(capability: str, db_types: list[str] | str | None = None):
        """
        Decorator to mark migrations as requiring specific database capabilities.

        Args:
            capability: Capability name (from CAPABILITIES dict)
            db_types: Database types that support this (None = check current DB)

        Example:
            @DatabaseCompat.requires("transaction_ddl")
            def migrate_with_transaction():
                # This migration requires transactional DDL
                pass

            @DatabaseCompat.requires("jsonb_support", ["postgres"])
            def migrate_jsonb_field():
                # This migration only works on PostgreSQL
                pass
        """
        import functools

        def decorator(func):
            @functools.wraps(func)
            def wrapper(*args, **kwargs):
                current_db = settings.DATABASE_TYPE.lower()

                # If specific db_types provided, check if current DB is in the list
                if db_types is not None:
                    allowed_dbs = [db_types] if isinstance(db_types, str) else db_types
                    allowed_dbs = [db.lower() for db in allowed_dbs]

                    if current_db not in allowed_dbs:
                        logging.warning(f"Migration {func.__name__} requires database type(s) {allowed_dbs}, current type is {current_db}. Skipping.")
                        return

                # Check if current database has the required capability
                if not DatabaseCompat.is_capable(current_db, capability):
                    logging.warning(f"Migration {func.__name__} requires capability '{capability}' which is not supported by {current_db}. Skipping.")
                    return

                # Capability is supported, execute migration
                return func(*args, **kwargs)

            return wrapper

        return decorator

    @staticmethod
    def db_specific(db_type: str):
        """
        Decorator to mark migrations as database-specific.

        Args:
            db_type: Database type this migration is for (mysql or postgres)

        Example:
            @DatabaseCompat.db_specific("mysql")
            def migrate_mysql_only():
                # This migration only runs on MySQL
                pass
        """
        import functools

        def decorator(func):
            @functools.wraps(func)
            def wrapper(*args, **kwargs):
                current_db = settings.DATABASE_TYPE.lower()
                target_db = db_type.lower()

                if current_db != target_db:
                    logging.debug(f"Migration {func.__name__} is for {target_db}, current database is {current_db}. Skipping.")
                    return

                return func(*args, **kwargs)

            return wrapper

        return decorator
