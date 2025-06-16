import opendal
import logging
import pymysql
import yaml

from rag.utils import singleton

SERVICE_CONF_PATH = "conf/service_conf.yaml"

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


def get_opendal_config_from_yaml(yaml_path=SERVICE_CONF_PATH):
    try:
        with open(yaml_path, 'r') as f:
            config = yaml.safe_load(f)

        opendal_config = config.get('opendal', {})
        kwargs = {}
        if opendal_config.get("scheme") == 'mysql':
            mysql_config = config.get('mysql', {})
            kwargs = {
                "scheme": "mysql",
                "host": mysql_config.get("host", "127.0.0.1"),
                "port": str(mysql_config.get("port", 3306)),
                "user": mysql_config.get("user", "root"),
                "password": mysql_config.get("password", ""),
                "database": mysql_config.get("name", "test_open_dal"),
                "table": opendal_config.get("config").get("table", "opendal_storage")
            }
            kwargs["connection_string"] = f"mysql://{kwargs['user']}:{kwargs['password']}@{kwargs['host']}:{kwargs['port']}/{kwargs['database']}"
        else:
            scheme = opendal_config.get("scheme")
            config_data = opendal_config.get("config", {})
            kwargs = {"scheme": scheme, **config_data}
        logging.info("Loaded OpenDAL configuration from yaml: %s", kwargs)
        return kwargs
    except Exception as e:
        logging.error("Failed to load OpenDAL configuration from yaml: %s", str(e))
        raise


@singleton
class OpenDALStorage:
    def __init__(self):
        self._kwargs = get_opendal_config_from_yaml()
        self._scheme = self._kwargs.get('scheme', 'mysql')
        if self._scheme == 'mysql':
            self.init_db_config()
            self.init_opendal_mysql_table()
        self._operator = opendal.Operator(**self._kwargs)

        logging.info("OpenDALStorage initialized successfully")

    def health(self):
        bucket, fnm, binary = "txtxtxtxt1", "txtxtxtxt1", b"_t@@@1"
        r = self._operator.write(f"{bucket}/{fnm}", binary)
        return r

    def put(self, bucket, fnm, binary):
        self._operator.write(f"{bucket}/{fnm}", binary)

    def get(self, bucket, fnm):
        return self._operator.read(f"{bucket}/{fnm}")

    def rm(self, bucket, fnm):
        self._operator.delete(f"{bucket}/{fnm}")
        self._operator.__init__()

    def scan(self, bucket, fnm):
        return self._operator.scan(f"{bucket}/{fnm}")

    def obj_exist(self, bucket, fnm):
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
            cursor.execute(SET_MAX_ALLOWED_PACKET_SQL.format(max_packet))
            conn.commit()
            cursor.close()
            conn.close()
            logging.info(f"Database configuration initialized with max_allowed_packet={max_packet}")
        except Exception as e:
            logging.error(f"Failed to initialize database configuration: {str(e)}")
            raise

    def init_opendal_mysql_table(self):
        conn = pymysql.connect(
            host=self._kwargs['host'],
            port=int(self._kwargs['port']),
            user=self._kwargs['user'],
            password=self._kwargs['password'],
            database=self._kwargs['database']
        )
        cursor = conn.cursor()
        cursor.execute(CREATE_TABLE_SQL.format(self._kwargs['table']))
        conn.commit()
        cursor.close()
        conn.close()
        logging.info(f"Table `{self._kwargs['table']}` initialized.")
