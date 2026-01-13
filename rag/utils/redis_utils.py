import asyncio
import uuid
from valkey.lock import Lock
from core.providers import providers


class RedisDistributedLock:
    def __init__(self, lock_key, lock_value=None, timeout=10, blocking_timeout=1):
        self.lock_key = lock_key
        if lock_value:
            self.lock_value = lock_value
        else:
            self.lock_value = str(uuid.uuid4())
        self.timeout = timeout
        self.lock = Lock(providers.cache.conn.REDIS, lock_key, timeout=timeout, blocking_timeout=blocking_timeout)

    def acquire(self):
        providers.cache.conn.delete_if_equal(self.lock_key, self.lock_value)
        return self.lock.acquire(token=self.lock_value)

    async def spin_acquire(self):
        providers.cache.conn.delete_if_equal(self.lock_key, self.lock_value)
        while True:
            if self.lock.acquire(token=self.lock_value):
                break
            await asyncio.sleep(10)

    def release(self):
        providers.cache.conn.delete_if_equal(self.lock_key, self.lock_value)
