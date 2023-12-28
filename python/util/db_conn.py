import logging
import time
from util import config
import pandas as pd


class Postgres(object):
    def __init__(self, env, dbnm):
        self.config = config.init(env)
        self.conn = None
        self.dbnm = dbnm
        self.__open__()

    def __open__(self):
        import psycopg2
        try:
            if self.conn:
                self.__close__()
            del self.conn
        except Exception as e:
            pass

        try:
            self.conn = psycopg2.connect(f"""dbname={self.dbnm}
                                         user={self.config.get('postgres_user')}
                                         password={self.config.get('postgres_password')}
                                         host={self.config.get('postgres_host')}
                                         port={self.config.get('postgres_port')}""")
        except Exception as e:
            logging.error(
                "Fail to connect %s " %
                self.config.get("pgdb_host") + str(e))

    def __close__(self):
        try:
            self.conn.close()
        except Exception as e:
            logging.error(
                "Fail to close %s " %
                self.config.get("pgdb_host") + str(e))

    def select(self, sql):
        for _ in range(10):
            try:
                return pd.read_sql(sql, self.conn)
            except Exception as e:
                logging.error(f"Fail to exec {sql}  " + str(e))
                self.__open__()
                time.sleep(1)

        return pd.DataFrame()

    def update(self, sql):
        for _ in range(10):
            try:
                cur = self.conn.cursor()
                cur.execute(sql)
                updated_rows = cur.rowcount
                self.conn.commit()
                cur.close()
                return updated_rows
            except Exception as e:
                logging.error(f"Fail to exec {sql}  " + str(e))
                self.__open__()
                time.sleep(1)
        return 0


if __name__ == "__main__":
    Postgres("infiniflow", "docgpt")
