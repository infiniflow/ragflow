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

import logging
import json
import uuid

import valkey as redis
from rag import settings
from rag.utils import singleton
from valkey.lock import Lock
import trio
import asyncio
import threading
import time
from typing import Optional, List, Dict, Any
from dataclasses import dataclass
from rag.utils.circuit_breaker import CircuitBreaker, CircuitBreakerConfig
from rag.utils.timeout_manager import get_timeout_manager


@dataclass
class RedisConnectionConfig:
    """Configuration for Redis connection resilience."""
    max_retries: int = 3
    retry_delay: float = 1.0
    connection_timeout: float = 5.0
    socket_timeout: float = 5.0
    health_check_interval: float = 30.0
    max_connections: int = 10
    enable_circuit_breaker: bool = True
    circuit_breaker_failure_threshold: int = 5
    circuit_breaker_recovery_timeout: float = 60.0


class RedisConnectionPool:
    """Enhanced Redis connection pool with resilience features."""

    def __init__(self, config: RedisConnectionConfig, redis_config: Dict[str, Any]):
        self.config = config
        self.redis_config = redis_config
        self._pool: Optional[redis.ConnectionPool] = None
        self._lock = threading.RLock()
        self._last_health_check = 0
        self._is_healthy = True

        # Circuit breaker for Redis operations
        if config.enable_circuit_breaker:
            cb_config = CircuitBreakerConfig(
                name="redis_operations",
                failure_threshold=config.circuit_breaker_failure_threshold,
                recovery_timeout=config.circuit_breaker_recovery_timeout,
                timeout=config.connection_timeout
            )
            self._circuit_breaker = CircuitBreaker(cb_config)
        else:
            self._circuit_breaker = None

        self._initialize_pool()

    def _initialize_pool(self):
        """Initialize the Redis connection pool."""
        try:
            pool_kwargs = {
                'host': self.redis_config["host"].split(":")[0],
                'port': int(self.redis_config.get("host", ":6379").split(":")[1]),
                'db': int(self.redis_config.get("db", 1)),
                'password': self.redis_config.get("password"),
                'decode_responses': True,
                'max_connections': self.config.max_connections,
                'socket_timeout': self.config.socket_timeout,
                'socket_connect_timeout': self.config.connection_timeout,
                'retry_on_timeout': True,
                'health_check_interval': self.config.health_check_interval
            }

            self._pool = redis.ConnectionPool(**pool_kwargs)
            logging.info(f"Redis connection pool initialized with {self.config.max_connections} max connections")

        except Exception as e:
            logging.error(f"Failed to initialize Redis connection pool: {e}")
            self._pool = None

    def get_connection(self) -> Optional[redis.Redis]:
        """Get a Redis connection from the pool."""
        with self._lock:
            if not self._pool:
                self._initialize_pool()

            if not self._pool:
                return None

            try:
                connection = redis.Redis(connection_pool=self._pool)

                # Perform health check if needed
                if time.time() - self._last_health_check > self.config.health_check_interval:
                    self._perform_health_check(connection)

                return connection

            except Exception as e:
                logging.error(f"Failed to get Redis connection: {e}")
                return None

    def _perform_health_check(self, connection: redis.Redis):
        """Perform health check on Redis connection."""
        try:
            connection.ping()
            self._is_healthy = True
            self._last_health_check = time.time()
            logging.debug("Redis health check passed")
        except Exception as e:
            self._is_healthy = False
            logging.warning(f"Redis health check failed: {e}")

    def is_healthy(self) -> bool:
        """Check if Redis connection pool is healthy."""
        return self._is_healthy

    def reset_pool(self):
        """Reset the connection pool."""
        with self._lock:
            if self._pool:
                try:
                    self._pool.disconnect()
                except Exception:
                    pass
            self._initialize_pool()


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
    LUA_DELETE_IF_EQUAL_SCRIPT = """
        local current_value = redis.call('get', KEYS[1])
        if current_value and current_value == ARGV[1] then
            redis.call('del', KEYS[1])
            return 1
        end
        return 0
    """

    def __init__(self):
        self.REDIS = None
        self.config = settings.REDIS

        # Enhanced resilience configuration
        self.resilience_config = RedisConnectionConfig()
        self._connection_pool: Optional[RedisConnectionPool] = None
        self._lock = threading.RLock()
        self._reconnect_attempts = 0
        self._max_reconnect_attempts = 5
        self._last_connection_attempt = 0
        self._connection_backoff = 1.0

        # Circuit breaker for Redis operations
        cb_config = CircuitBreakerConfig(
            name="redis_db_operations",
            failure_threshold=5,
            recovery_timeout=60.0,
            timeout=30.0
        )
        self._circuit_breaker = CircuitBreaker(cb_config)

        self.__open__()

    def register_scripts(self) -> None:
        cls = self.__class__
        client = self.REDIS
        cls.lua_delete_if_equal = client.register_script(cls.LUA_DELETE_IF_EQUAL_SCRIPT)

    def __open__(self):
        """Open Redis connection with enhanced resilience."""
        with self._lock:
            current_time = time.time()

            # Implement connection backoff
            if (self._last_connection_attempt > 0 and
                current_time - self._last_connection_attempt < self._connection_backoff):
                logging.debug(f"Connection backoff active, waiting {self._connection_backoff}s")
                return self.REDIS

            self._last_connection_attempt = current_time

            try:
                # Initialize connection pool if not exists
                if not self._connection_pool:
                    self._connection_pool = RedisConnectionPool(
                        self.resilience_config,
                        self.config
                    )

                # Get connection from pool
                self.REDIS = self._connection_pool.get_connection()

                if self.REDIS:
                    self.register_scripts()
                    self._reconnect_attempts = 0
                    self._connection_backoff = 1.0
                    logging.info("Redis connection established successfully")
                else:
                    raise Exception("Failed to get connection from pool")

            except Exception as e:
                self._reconnect_attempts += 1
                self._connection_backoff = min(30.0, self._connection_backoff * 2)  # Exponential backoff

                logging.warning(
                    f"Redis connection failed (attempt {self._reconnect_attempts}): {e}. "
                    f"Next attempt in {self._connection_backoff}s"
                )

                if self._reconnect_attempts >= self._max_reconnect_attempts:
                    logging.error(f"Redis connection failed after {self._max_reconnect_attempts} attempts")
                    self.REDIS = None

        return self.REDIS

    def health(self):
        self.REDIS.ping()
        a, b = "xx", "yy"
        self.REDIS.set(a, b, 3)

        if self.REDIS.get(a) == b:
            return True

    def is_alive(self):
        return self.REDIS is not None

    def _execute_with_resilience(self, operation_name: str, operation_func, *args, **kwargs):
        """Execute Redis operation with resilience features."""
        if not self.REDIS:
            self.__open__()
            if not self.REDIS:
                logging.error(f"Redis operation '{operation_name}' failed: No connection available")
                return None

        try:
            # Use circuit breaker for operation
            return self._circuit_breaker.call(operation_func, *args, **kwargs)

        except Exception as e:
            logging.warning(f"RedisDB.{operation_name} got exception: {e}")

            # Attempt reconnection on connection errors
            if "connection" in str(e).lower() or "timeout" in str(e).lower():
                logging.info(f"Attempting Redis reconnection due to: {e}")
                self.__open__()

                # Retry operation once after reconnection
                if self.REDIS:
                    try:
                        return self._circuit_breaker.call(operation_func, *args, **kwargs)
                    except Exception as retry_e:
                        logging.error(f"RedisDB.{operation_name} retry failed: {retry_e}")

            return None

    def exist(self, k):
        if not self.REDIS:
            return

        def _exist_operation():
            return self.REDIS.exists(k)

        return self._execute_with_resilience("exist", _exist_operation)

    def get(self, k):
        if not self.REDIS:
            return

        def _get_operation():
            return self.REDIS.get(k)

        return self._execute_with_resilience("get", _get_operation)

    def set_obj(self, k, obj, exp=3600):
        def _set_obj_operation():
            self.REDIS.set(k, json.dumps(obj, ensure_ascii=False), exp)
            return True

        result = self._execute_with_resilience("set_obj", _set_obj_operation)
        return result if result is not None else False

    def set(self, k, v, exp=3600):
        def _set_operation():
            self.REDIS.set(k, v, exp)
            return True

        result = self._execute_with_resilience("set", _set_operation)
        return result if result is not None else False

    def sadd(self, key: str, member: str):
        try:
            self.REDIS.sadd(key, member)
            return True
        except Exception as e:
            logging.warning("RedisDB.sadd " + str(key) + " got exception: " + str(e))
            self.__open__()
        return False

    def srem(self, key: str, member: str):
        try:
            self.REDIS.srem(key, member)
            return True
        except Exception as e:
            logging.warning("RedisDB.srem " + str(key) + " got exception: " + str(e))
            self.__open__()
        return False

    def smembers(self, key: str):
        try:
            res = self.REDIS.smembers(key)
            return res
        except Exception as e:
            logging.warning(
                "RedisDB.smembers " + str(key) + " got exception: " + str(e)
            )
            self.__open__()
        return None

    def zadd(self, key: str, member: str, score: float):
        try:
            self.REDIS.zadd(key, {member: score})
            return True
        except Exception as e:
            logging.warning("RedisDB.zadd " + str(key) + " got exception: " + str(e))
            self.__open__()
        return False

    def zcount(self, key: str, min: float, max: float):
        try:
            res = self.REDIS.zcount(key, min, max)
            return res
        except Exception as e:
            logging.warning("RedisDB.zcount " + str(key) + " got exception: " + str(e))
            self.__open__()
        return 0

    def zpopmin(self, key: str, count: int):
        try:
            res = self.REDIS.zpopmin(key, count)
            return res
        except Exception as e:
            logging.warning("RedisDB.zpopmin " + str(key) + " got exception: " + str(e))
            self.__open__()
        return None

    def zrangebyscore(self, key: str, min: float, max: float):
        try:
            res = self.REDIS.zrangebyscore(key, min, max)
            return res
        except Exception as e:
            logging.warning(
                "RedisDB.zrangebyscore " + str(key) + " got exception: " + str(e)
            )
            self.__open__()
        return None

    def transaction(self, key, value, exp=3600):
        try:
            pipeline = self.REDIS.pipeline(transaction=True)
            pipeline.set(key, value, exp, nx=True)
            pipeline.execute()
            return True
        except Exception as e:
            logging.warning(
                "RedisDB.transaction " + str(key) + " got exception: " + str(e)
            )
            self.__open__()
        return False

    def queue_product(self, queue, message) -> bool:
        def _queue_product_operation():
            payload = {"message": json.dumps(message)}
            self.REDIS.xadd(queue, payload)
            return True

        # Retry with resilience
        for attempt in range(self.resilience_config.max_retries):
            result = self._execute_with_resilience("queue_product", _queue_product_operation)
            if result:
                return True

            if attempt < self.resilience_config.max_retries - 1:
                time.sleep(self.resilience_config.retry_delay * (2 ** attempt))

        return False

    def queue_consumer(self, queue_name, group_name, consumer_name, msg_id=b">") -> RedisMsg:
        """https://redis.io/docs/latest/commands/xreadgroup/"""
        try:
            group_info = self.REDIS.xinfo_groups(queue_name)
            if not any(gi["name"] == group_name for gi in group_info):
                self.REDIS.xgroup_create(queue_name, group_name, id="0", mkstream=True)
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
        except Exception as e:
            if str(e) == 'no such key':
                pass
            else:
                logging.exception(
                    "RedisDB.queue_consumer "
                    + str(queue_name)
                    + " got exception: "
                    + str(e)
                )
        return None

    def get_unacked_iterator(self, queue_names: list[str], group_name, consumer_name):
        try:
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
        except Exception:
            logging.exception(
                "RedisDB.get_unacked_iterator got exception: "
            )
            self.__open__()

    def get_pending_msg(self, queue, group_name):
        try:
            messages = self.REDIS.xpending_range(queue, group_name, '-', '+', 10)
            return messages
        except Exception as e:
            if 'No such key' not in (str(e) or ''):
                logging.warning(
                    "RedisDB.get_pending_msg " + str(queue) + " got exception: " + str(e)
                )
        return []

    def requeue_msg(self, queue: str, group_name: str, msg_id: str):
        try:
            messages = self.REDIS.xrange(queue, msg_id, msg_id)
            if messages:
                self.REDIS.xadd(queue, messages[0][1])
                self.REDIS.xack(queue, group_name, msg_id)
        except Exception as e:
            logging.warning(
                "RedisDB.get_pending_msg " + str(queue) + " got exception: " + str(e)
            )

    def queue_info(self, queue, group_name) -> dict | None:
        try:
            groups = self.REDIS.xinfo_groups(queue)
            for group in groups:
                if group["name"] == group_name:
                    return group
        except Exception as e:
            logging.warning(
                "RedisDB.queue_info " + str(queue) + " got exception: " + str(e)
            )
        return None

    def delete_if_equal(self, key: str, expected_value: str) -> bool:
        """
        Do follwing atomically:
        Delete a key if its value is equals to the given one, do nothing otherwise.
        """
        return bool(self.lua_delete_if_equal(keys=[key], args=[expected_value], client=self.REDIS))

    def delete(self, key) -> bool:
        try:
            self.REDIS.delete(key)
            return True
        except Exception as e:
            logging.warning("RedisDB.delete " + str(key) + " got exception: " + str(e))
            self.__open__()
        return False
    
    
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
            await trio.sleep(10)

    def release(self):
        REDIS_CONN.delete_if_equal(self.lock_key, self.lock_value)
