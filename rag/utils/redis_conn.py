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

import asyncio
import logging
import json
import uuid
import functools

import valkey as redis
from common.decorator import singleton
from common import settings
from valkey.lock import Lock

REDIS = {}
try:
    REDIS = settings.decrypt_database_config(name="redis")
except Exception:
    try:
        REDIS = settings.get_base_config("redis", {})
    except Exception:
        REDIS = {}


def redis_reconnect_decorator(default_return=None):
    """
    Decorator to handle Redis exceptions and reconnect logic
    
    Args:
        default_return: Default value to return when an exception occurs
    """
    def decorator(func):
        @functools.wraps(func)
        def wrapper(self, *args, **kwargs):
            try:
                if not self.REDIS:
                    self.__open__()
                    if not self.REDIS:
                        return default_return
                return func(self, *args, **kwargs)
            except Exception as e:
                method_name = func.__name__
                logging.warning(f"RedisDB.{method_name} got exception: {e}")
                self.__open__()
                return default_return
        return wrapper
    return decorator


def redis_retry_decorator(max_retries=3, default_return=None):
    """
    Decorator to add retry logic for Redis operations
    
    Args:
        max_retries: Maximum number of retry attempts
        default_return: Default value to return when all retries fail
    """
    def decorator(func):
        @functools.wraps(func)
        def wrapper(self, *args, **kwargs):
            for _ in range(max_retries):
                try:
                    if not self.REDIS:
                        self.__open__()
                    return func(self, *args, **kwargs)
                except Exception as e:
                    method_name = func.__name__
                    logging.warning(f"RedisDB.{method_name} got exception: {e}")
                    self.__open__()
            return default_return
        return wrapper
    return decorator


class RedisMsg:
    def __init__(self, consumer, queue_name, group_name, msg_id, message):
        self.__consumer = consumer
        self.__queue_name = queue_name
        self.__group_name = group_name
        self.__msg_id = msg_id
        self.__message = json.loads(message["message"])

    def ack(self):
        try:
            self.__consumer.xack(self.__queue_name, self.__group_name, self.__msg_id)
            return True
        except Exception as e:
            logging.warning("[EXCEPTION]ack" + str(self.__queue_name) + "||" + str(e))
        return False

    def get_message(self):
        return self.__message

    def get_msg_id(self):
        return self.__msg_id


@singleton
class RedisDB:
    lua_delete_if_equal = None
    lua_token_bucket = None
    LUA_DELETE_IF_EQUAL_SCRIPT = """
        local current_value = redis.call('get', KEYS[1])
        if current_value and current_value == ARGV[1] then
            redis.call('del', KEYS[1])
            return 1
        end
        return 0
    """

    LUA_TOKEN_BUCKET_SCRIPT = """
        -- KEYS[1] = rate limit key
        -- ARGV[1] = capacity
        -- ARGV[2] = rate
        -- ARGV[3] = now
        -- ARGV[4] = cost

        local key       = KEYS[1]
        local capacity  = tonumber(ARGV[1])
        local rate      = tonumber(ARGV[2])
        local now       = tonumber(ARGV[3])
        local cost      = tonumber(ARGV[4])

        local data = redis.call("HMGET", key, "tokens", "timestamp")
        local tokens = tonumber(data[1])
        local last_ts = tonumber(data[2])

        if tokens == nil then
            tokens = capacity
            last_ts = now
        end

        local delta = math.max(0, now - last_ts)
        tokens = math.min(capacity, tokens + delta * rate)

        if tokens < cost then
            return {0, tokens}
        end

        tokens = tokens - cost

        redis.call("HMSET", key,
            "tokens", tokens,
            "timestamp", now
        )

        redis.call("EXPIRE", key, math.ceil(capacity / rate * 2))

        return {1, tokens}
    """

    def __init__(self):
        self.REDIS = None
        self.config = REDIS
        self.__open__()

    def register_scripts(self) -> None:
        cls = self.__class__
        client = self.REDIS
        cls.lua_delete_if_equal = client.register_script(cls.LUA_DELETE_IF_EQUAL_SCRIPT)
        cls.lua_token_bucket = client.register_script(cls.LUA_TOKEN_BUCKET_SCRIPT)

    def __open__(self):
        try:
            conn_params = {
                "host": self.config["host"].split(":")[0],
                "port": int(self.config.get("host", ":6379").split(":")[1]),
                "db": int(self.config.get("db", 1)),
                "decode_responses": True,
            }
            username = self.config.get("username")
            if username:
                conn_params["username"] = username
            password = self.config.get("password")
            if password:
                conn_params["password"] = password

            self.REDIS = redis.StrictRedis(**conn_params)

            self.register_scripts()
        except Exception as e:
            logging.warning(f"Redis can't be connected. Error: {str(e)}")
        return self.REDIS

    def health(self):
        self.REDIS.ping()
        a, b = "xx", "yy"
        self.REDIS.set(a, b, 3)

        if self.REDIS.get(a) == b:
            return True
        return False

    def info(self):
        info = self.REDIS.info()
        return {
            'redis_version': info["redis_version"],
            'server_mode': info["server_mode"] if "server_mode" in info else info.get("redis_mode", ""),
            'used_memory': info["used_memory_human"],
            'total_system_memory': info["total_system_memory_human"],
            'mem_fragmentation_ratio': info["mem_fragmentation_ratio"],
            'connected_clients': info["connected_clients"],
            'blocked_clients': info["blocked_clients"],
            'instantaneous_ops_per_sec': info["instantaneous_ops_per_sec"],
            'total_commands_processed': info["total_commands_processed"]
        }

    def is_alive(self):
        return self.REDIS is not None

    @redis_reconnect_decorator(default_return=None)
    def exist(self, k):
        return self.REDIS.exists(k)

    @redis_reconnect_decorator(default_return=None)
    def get(self, k):
        return self.REDIS.get(k)

    @redis_reconnect_decorator(default_return=False)
    def set_obj(self, k, obj, exp=3600):
        self.REDIS.set(k, json.dumps(obj, ensure_ascii=False), exp)
        return True

    @redis_reconnect_decorator(default_return=False)
    def set(self, k, v, exp=3600):
        self.REDIS.set(k, v, exp)
        return True

    @redis_reconnect_decorator(default_return=False)
    def sadd(self, key: str, member: str):
        self.REDIS.sadd(key, member)
        return True

    @redis_reconnect_decorator(default_return=False)
    def srem(self, key: str, member: str):
        self.REDIS.srem(key, member)
        return True

    @redis_reconnect_decorator(default_return=None)
    def smembers(self, key: str):
        return self.REDIS.smembers(key)

    @redis_reconnect_decorator(default_return=False)
    def zadd(self, key: str, member: str, score: float):
        self.REDIS.zadd(key, {member: score})
        return True

    @redis_reconnect_decorator(default_return=0)
    def zcount(self, key: str, min: float, max: float):
        return self.REDIS.zcount(key, min, max)

    @redis_reconnect_decorator(default_return=None)
    def zpopmin(self, key: str, count: int):
        return self.REDIS.zpopmin(key, count)

    @redis_reconnect_decorator(default_return=None)
    def zrangebyscore(self, key: str, min: float, max: float):
        return self.REDIS.zrangebyscore(key, min, max)

    @redis_reconnect_decorator(default_return=0)
    def zremrangebyscore(self, key: str, min: float, max: float):
        return self.REDIS.zremrangebyscore(key, min, max)

    @redis_reconnect_decorator(default_return=0)
    def incrby(self, key: str, increment: int):
        return self.REDIS.incrby(key, increment)

    @redis_reconnect_decorator(default_return=0)
    def decrby(self, key: str, decrement: int):
        return self.REDIS.decrby(key, decrement)

    @redis_reconnect_decorator(default_return=-1)
    def generate_auto_increment_id(self, key_prefix: str = "id_generator", namespace: str = "default",
                                   increment: int = 1, ensure_minimum: int | None = None) -> int:
        redis_key = f"{key_prefix}:{namespace}"

        # Use pipeline for atomicity
        pipe = self.REDIS.pipeline()

        # Check if key exists
        pipe.exists(redis_key)

        # Get/Increment
        if ensure_minimum is not None:
            # Ensure minimum value
            pipe.get(redis_key)
            results = pipe.execute()

            if results[0] == 0:  # Key doesn't exist
                start_id = max(1, ensure_minimum)
                pipe.set(redis_key, start_id)
                pipe.execute()
                return start_id
            else:
                current = int(results[1])
                if current < ensure_minimum:
                    pipe.set(redis_key, ensure_minimum)
                    pipe.execute()
                    return ensure_minimum

        # Increment operation
        next_id = self.REDIS.incrby(redis_key, increment)

        # If it's the first time, set a reasonable initial value
        if next_id == increment:
            self.REDIS.set(redis_key, 1 + increment)
            return 1 + increment

        return next_id

    @redis_reconnect_decorator(default_return=False)
    def transaction(self, key, value, exp=3600):
        pipeline = self.REDIS.pipeline(transaction=True)
        pipeline.set(key, value, exp, nx=True)
        pipeline.execute()
        return True

    @redis_retry_decorator(default_return=False)
    def queue_product(self, queue, message) -> bool:
        payload = {"message": json.dumps(message)}
        self.REDIS.xadd(queue, payload)
        return True

    @redis_retry_decorator(default_return=None)
    def queue_consumer(self, queue_name, group_name, consumer_name, msg_id=b">") -> RedisMsg:
        """https://redis.io/docs/latest/commands/xreadgroup/"""
        try:
            group_info = self.REDIS.xinfo_groups(queue_name)
            if not any(gi["name"] == group_name for gi in group_info):
                self.REDIS.xgroup_create(queue_name, group_name, id="0", mkstream=True)
        except redis.exceptions.ResponseError as e:
            if "no such key" in str(e).lower():
                self.REDIS.xgroup_create(queue_name, group_name, id="0", mkstream=True)
            elif "busygroup" in str(e).lower():
                logging.warning("Group already exists, continue.")
                pass
            else:
                raise

        args = {
            "groupname": group_name,
            "consumername": consumer_name,
            "count": 1,
            "block": 5,
            "streams": {queue_name: msg_id},
        }
        messages = self.REDIS.xreadgroup(**args)
        if not messages:
            return None
        stream, element_list = messages[0]
        if not element_list:
            return None
        msg_id, payload = element_list[0]
        res = RedisMsg(self.REDIS, queue_name, group_name, msg_id, payload)
        return res

    @redis_reconnect_decorator(default_return=None)
    def get_unacked_iterator(self, queue_names: list[str], group_name, consumer_name):
        for queue_name in queue_names:
            try:
                group_info = self.REDIS.xinfo_groups(queue_name)
            except Exception as e:
                if str(e) == 'no such key':
                    logging.warning(f"RedisDB.get_unacked_iterator queue {queue_name} doesn't exist")
                    continue
            if not any(gi["name"] == group_name for gi in group_info):
                logging.warning(f"RedisDB.get_unacked_iterator queue {queue_name} group {group_name} doesn't exist")
                continue
            current_min = 0
            while True:
                payload = self.queue_consumer(queue_name, group_name, consumer_name, current_min)
                if not payload:
                    break
                current_min = payload.get_msg_id()
                logging.info(f"RedisDB.get_unacked_iterator {queue_name} {consumer_name} {current_min}")
                yield payload

    @redis_reconnect_decorator(default_return=[])
    def get_pending_msg(self, queue, group_name):
        try:
            messages = self.REDIS.xpending_range(queue, group_name, '-', '+', 10)
            return messages
        except Exception as e:
            if 'No such key' not in (str(e) or ''):
                logging.warning(
                    f"RedisDB.get_pending_msg {queue} got exception: {e}"
                )
            return []

    @redis_retry_decorator(default_return=None)
    def requeue_msg(self, queue: str, group_name: str, msg_id: str):
        messages = self.REDIS.xrange(queue, msg_id, msg_id)
        if messages:
            self.REDIS.xadd(queue, messages[0][1])
            self.REDIS.xack(queue, group_name, msg_id)

    @redis_retry_decorator(default_return=None)
    def queue_info(self, queue, group_name) -> dict | None:
        groups = self.REDIS.xinfo_groups(queue)
        for group in groups:
            if group["name"] == group_name:
                return group
        return None

    @redis_reconnect_decorator(default_return=False)
    def delete_if_equal(self, key: str, expected_value: str) -> bool:
        """
        Do following atomically:
        Delete a key if its value is equals to the given one, do nothing otherwise.
        """
        return bool(self.lua_delete_if_equal(keys=[key], args=[expected_value], client=self.REDIS))

    @redis_reconnect_decorator(default_return=False)
    def delete(self, key) -> bool:
        self.REDIS.delete(key)
        return True


REDIS_CONN = RedisDB()


class RedisDistributedLock:
    def __init__(self, lock_key, lock_value=None, timeout=10, blocking_timeout=1):
        self.lock_key = lock_key
        if lock_value:
            self.lock_value = lock_value
        else:
            self.lock_value = str(uuid.uuid4())
        self.timeout = timeout
        self.lock = Lock(REDIS_CONN.REDIS, lock_key, timeout=timeout, blocking_timeout=blocking_timeout)

    def acquire(self):
        REDIS_CONN.delete_if_equal(self.lock_key, self.lock_value)
        return self.lock.acquire(token=self.lock_value)

    async def spin_acquire(self):
        REDIS_CONN.delete_if_equal(self.lock_key, self.lock_value)
        while True:
            if self.lock.acquire(token=self.lock_value):
                break
            await asyncio.sleep(10)

    def release(self):
        REDIS_CONN.delete_if_equal(self.lock_key, self.lock_value)
