import opendal
import logging
import pymysql
import re
from urllib.parse import quote_plus

from common.config_utils import get_base_config
from common.decorator import singleton

CREATE_TABLE_SQL = """
CREATE TABLE IF NOT EXISTS `{}` (
    `key` VARCHAR(255) PRIMARY KEY,
    `value` LONGBLOB,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
"""
SET_MAX_ALLOWED_PACKET_SQL = """
SET GLOBAL max_allowed_packet={}
"""


def get_opendal_config():
    try:
        opendal_config = get_base_config('opendal', {})
        if opendal_config.get("scheme", "mysql") == 'mysql':
            mysql_config = get_base_config('mysql', {})
            max_packet = mysql_config.get("max_allowed_packet", 134217728)
            kwargs = {
                "scheme": "mysql",
                "host": mysql_config.get("host", "127.0.0.1"),
                "port": str(mysql_config.get("port", 3306)),
                "user": mysql_config.get("user", "root"),
                "password": mysql_config.get("password", ""),
                "database": mysql_config.get("name", "test_open_dal"),
                "table": opendal_config.get("config", {}).get("oss_table", "opendal_storage"),
                "max_allowed_packet": str(max_packet)
            }
            kwargs[
                "connection_string"] = f"mysql://{kwargs['user']}:{quote_plus(kwargs['password'])}@{kwargs['host']}:{kwargs['port']}/{kwargs['database']}?max_allowed_packet={max_packet}"
        else:
            scheme = opendal_config.get("scheme")
            config_data = opendal_config.get("config", {})
            kwargs = {"scheme": scheme, **config_data}

        # Only include non-sensitive keys in logs. Do NOT
        # add 'password' or any key containing embedded credentials
        # (like 'connection_string').
        safe_log_info = {
            "scheme": kwargs.get("scheme"),
            "host": kwargs.get("host"),
            "port": kwargs.get("port"),
            "database": kwargs.get("database"),
            "table": kwargs.get("table"),
            # indicate presence of credentials without logging them
            "has_credentials": any(k in kwargs for k in ("password", "connection_string")),
        }
        logging.info("Loaded OpenDAL configuration (non sensitive fields only): %s", safe_log_info)
        return kwargs
    except Exception as e:
        logging.error("Failed to load OpenDAL configuration from yaml: %s", str(e))
        raise


@singleton
class OpenDALStorage:
    def __init__(self):
        self._kwargs = get_opendal_config()
        self._scheme = self._kwargs.get('scheme', 'mysql')
        if self._scheme == 'mysql':
            self.init_db_config()
            self.init_opendal_mysql_table()
        self._operator = opendal.Operator(**self._kwargs)

        logging.info("OpenDALStorage initialized successfully")

    def health(self):
        bucket, fnm, binary = "txtxtxtxt1", "txtxtxtxt1", b"_t@@@1"
        return self._operator.write(f"{bucket}/{fnm}", binary)

    def put(self, bucket, fnm, binary, tenant_id=None):
        self._operator.write(f"{bucket}/{fnm}", binary)

    def get(self, bucket, fnm, tenant_id=None):
        return self._operator.read(f"{bucket}/{fnm}")

    def rm(self, bucket, fnm, tenant_id=None):
        self._operator.delete(f"{bucket}/{fnm}")
        self._operator.__init__()

    def scan(self, bucket, fnm, tenant_id=None):
        return self._operator.scan(f"{bucket}/{fnm}")

    def obj_exist(self, bucket, fnm, tenant_id=None):
        return self._operator.exists(f"{bucket}/{fnm}")

    def init_db_config(self):
        try:
            conn = pymysql.connect(
                host=self._kwargs['host'],
                port=int(self._kwargs['port']),
                user=self._kwargs['user'],
                password=self._kwargs['password'],
                database=self._kwargs['database']
            )
            cursor = conn.cursor()
            max_packet = self._kwargs.get('max_allowed_packet', 4194304)  # Default to 4MB if not specified
            # Ensure max_packet is a valid integer to prevent SQL injection
            cursor.execute(SET_MAX_ALLOWED_PACKET_SQL.format(int(max_packet)))
            conn.commit()
            cursor.close()
            conn.close()
            logging.info(f"Database configuration initialized with max_allowed_packet={max_packet}")
        except Exception as e:
            logging.error(f"Failed to initialize database configuration: {str(e)}")
            raise

    def init_opendal_mysql_table(self):
        table_name = self._kwargs['table']
        # Validate table name to prevent SQL injection
        if not re.match(r'^[a-zA-Z0-9_]+$', table_name):
            raise ValueError(f"Invalid table name: {table_name}")

        conn = pymysql.connect(
            host=self._kwargs['host'],
            port=int(self._kwargs['port']),
            user=self._kwargs['user'],
            password=self._kwargs['password'],
            database=self._kwargs['database']
        )
        cursor = conn.cursor()
        cursor.execute(CREATE_TABLE_SQL.format(table_name))
        conn.commit()
        cursor.close()
        conn.close()
        logging.info(f"Table `{table_name}` initialized.")
