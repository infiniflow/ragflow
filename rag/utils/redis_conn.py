import json

import redis
import logging
from rag import settings
from rag.utils import singleton


class Payload:
    def __init__(self, consumer, queue_name, group_name, msg_id, message):
        self.__consumer = consumer
        self.__queue_name = queue_name
        self.__group_name = group_name
        self.__msg_id = msg_id
        self.__message = json.loads(message['message'])

    def ack(self):
        try:
            self.__consumer.xack(self.__queue_name, self.__group_name, self.__msg_id)
            return True
        except Exception as e:
            logging.warning("[EXCEPTION]ack" + str(self.__queue_name) + "||" + str(e))
        return False

    def get_message(self):
        return self.__message


@singleton
class RedisDB:
    def __init__(self):
        self.REDIS = None
        self.config = settings.REDIS
        self.__open__()

    def __open__(self):
        try:
            self.REDIS = redis.StrictRedis(host=self.config["host"].split(":")[0],
                                     port=int(self.config.get("host", ":6379").split(":")[1]),
                                     db=int(self.config.get("db", 1)),
                                     password=self.config.get("password"),
                                     decode_responses=True)
        except Exception as e:
            logging.warning("Redis can't be connected.")
        return self.REDIS

    def health(self):

        self.REDIS.ping()
        a, b = 'xx', 'yy'
        self.REDIS.set(a, b, 3)

        if self.REDIS.get(a) == b:
            return True

    def is_alive(self):
        return self.REDIS is not None

    def exist(self, k):
        if not self.REDIS: return
        try:
            return self.REDIS.exists(k)
        except Exception as e:
            logging.warning("[EXCEPTION]exist" + str(k) + "||" + str(e))
            self.__open__()

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

    def transaction(self, key, value, exp=3600):
        try:
            pipeline = self.REDIS.pipeline(transaction=True)
            pipeline.set(key, value, exp, nx=True)
            pipeline.execute()
            return True
        except Exception as e:
            logging.warning("[EXCEPTION]set" + str(key) + "||" + str(e))
            self.__open__()
        return False

    def queue_product(self, queue, message, exp=settings.SVR_QUEUE_RETENTION) -> bool:
        for _ in range(3):
            try:
                payload = {"message": json.dumps(message)}
                pipeline = self.REDIS.pipeline()
                pipeline.xadd(queue, payload)
                pipeline.expire(queue, exp)
                pipeline.execute()
                return True
            except Exception as e:
                print(e)
                logging.warning("[EXCEPTION]producer" + str(queue) + "||" + str(e))
        return False

    def queue_consumer(self, queue_name, group_name, consumer_name, msg_id=b">") -> Payload:
        try:
            group_info = self.REDIS.xinfo_groups(queue_name)
            if not any(e["name"] == group_name for e in group_info):
                self.REDIS.xgroup_create(
                    queue_name,
                    group_name,
                    id="0",
                    mkstream=True
                )
            args = {
                "groupname": group_name,
                "consumername": consumer_name,
                "count": 1,
                "block": 10000,
                "streams": {queue_name: msg_id},
            }
            messages = self.REDIS.xreadgroup(**args)
            if not messages:
                return None
            stream, element_list = messages[0]
            msg_id, payload = element_list[0]
            res = Payload(self.REDIS, queue_name, group_name, msg_id, payload)
            return res
        except Exception as e:
            if 'key' in str(e):
                pass
            else:
                logging.warning("[EXCEPTION]consumer" + str(queue_name) + "||" + str(e))
        return None


REDIS_CONN = RedisDB()
