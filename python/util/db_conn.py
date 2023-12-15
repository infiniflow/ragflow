import logging
import time
from util import config
import pandas as pd

class Postgre(object):
    def __init__(self, env, dbnm):
        self.config = config.init(env)
        self.conn = None
        self.dbnm = dbnm
        self.__open__()

    def __open__(self):
        import psycopg2
        try:
            if self.conn:self.__close__()
            del self.conn
        except Exception as e:
            pass

        try:
            self.conn = psycopg2.connect(f"dbname={self.dbnm} user={self.config.get('pgdb_usr')} password={self.config.get('pgdb_pwd')} host={self.config.get('pgdb_host')} port={self.config.get('pgdb_port')}")
        except Exception as e:
            logging.error("Fail to connect %s "%self.config.get("pgdb_host") + str(e))


    def __close__(self):
        try:
            self.conn.close()
        except Exception as e:
             logging.error("Fail to close %s "%self.config.get("pgdb_host") + str(e))


    def select(self, sql):
        for _ in range(10):
            try:
                return pd.read_sql(sql, self.conn)
            except Exception as e:
                logging.error(f"Fail to exec {sql}l  "+str(e))
                self.__open__()
                time.sleep(1)

        return pd.DataFrame()

