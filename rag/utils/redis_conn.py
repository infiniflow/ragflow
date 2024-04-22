import json

import redis
import logging
from rag import settings
from rag.utils import singleton

@singleton
class RedisDB:
    def __init__(self):
        self.REDIS = None
        self.config = settings.REDIS
        self.__open__()

    def __open__(self):
        try:
            self.REDIS = redis.Redis(host=self.config.get("host", "redis").split(":")[0],
                                     port=int(self.config.get("host", ":6379").split(":")[1]),
                                     db=int(self.config.get("db", 1)),
                                     password=self.config.get("password"))
        except Exception as e:
            logging.warning("Redis can't be connected.")
        return self.REDIS

    def is_alive(self):
        return self.REDIS is not None

    def get(self, k):
        if not self.REDIS: return
        try:
            return self.REDIS.get(k)
        except Exception as e:
            logging.warning("[EXCEPTION]get" + str(k) + "||" + str(e))
            self.__open__()

    def set_obj(self, k, obj, exp=3600):
        try:
            self.REDIS.set(k, json.dumps(obj, ensure_ascii=False), exp)
            return True
        except Exception as e:
            logging.warning("[EXCEPTION]set_obj" + str(k) + "||" + str(e))
            self.__open__()
        return False

    def set(self, k, v, exp=3600):
        try:
            self.REDIS.set(k, v, exp)
            return True
        except Exception as e:
            logging.warning("[EXCEPTION]set" + str(k) + "||" + str(e))
            self.__open__()
        return False


REDIS_CONN = RedisDB()