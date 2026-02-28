#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

import json
import logging
import random
import time

from api.db import CanvasCategory
from rag.utils.redis_conn import REDIS_CONN, RedisDistributedLock


class CanvasReplicaService:
    """
    Manage per-user canvas runtime replicas stored in Redis.

    Lifecycle:
    - bootstrap: initialize/refresh replica from DB DSL
    - load_for_run: read replica before run
    - commit_after_run: atomically persist run result back to replica
    """

    TTL_SECS = 3 * 60 * 60
    REPLICA_KEY_PREFIX = "canvas:replica"
    LOCK_KEY_PREFIX = "canvas:replica:lock"
    LOCK_TIMEOUT_SECS = 10
    LOCK_BLOCKING_TIMEOUT_SECS = 1
    LOCK_RETRY_ATTEMPTS = 3
    LOCK_RETRY_SLEEP_SECS = 0.2


    @classmethod
    def normalize_dsl(cls, dsl):
        """Normalize DSL to a JSON-serializable dict. Raise ValueError on invalid input."""
        normalized = dsl
        if isinstance(normalized, str):
            try:
                normalized = json.loads(normalized)
            except Exception as e:
                raise ValueError("Invalid DSL JSON string.") from e

        if not isinstance(normalized, dict):
            raise ValueError("DSL must be a JSON object.")

        try:
            return json.loads(json.dumps(normalized, ensure_ascii=False))
        except Exception as e:
            raise ValueError("DSL is not JSON-serializable.") from e


    @classmethod
    def _replica_key(cls, canvas_id: str, tenant_id: str, runtime_user_id: str) -> str:
        return f"{cls.REPLICA_KEY_PREFIX}:{canvas_id}:{tenant_id}:{runtime_user_id}"


    @classmethod
    def _lock_key(cls, canvas_id: str, tenant_id: str, runtime_user_id: str) -> str:
        return f"{cls.LOCK_KEY_PREFIX}:{canvas_id}:{tenant_id}:{runtime_user_id}"


    @classmethod
    def _read_payload(cls, replica_key: str):
        """Read replica payload from Redis; return None on missing/invalid content."""
        cache_blob = REDIS_CONN.get(replica_key)
        if not cache_blob:
            return None
        try:
            payload = json.loads(cache_blob)
            if not isinstance(payload, dict):
                return None
            payload["dsl"] = cls.normalize_dsl(payload.get("dsl", {}))
            return payload
        except Exception as e:
            logging.warning("Failed to parse canvas replica %s: %s", replica_key, e)
            return None


    @classmethod
    def _write_payload(cls, replica_key: str, payload: dict):
        """Write payload and refresh TTL."""
        payload["updated_at"] = int(time.time())
        REDIS_CONN.set_obj(replica_key, payload, cls.TTL_SECS)


    @classmethod
    def _build_payload(
        cls,
        canvas_id: str,
        tenant_id: str,
        runtime_user_id: str,
        dsl,
        canvas_category=CanvasCategory.Agent,
        title="",
    ):
        return {
            "canvas_id": canvas_id,
            "tenant_id": str(tenant_id),
            "runtime_user_id": str(runtime_user_id),
            "title": title or "",
            "canvas_category": canvas_category or CanvasCategory.Agent,
            "dsl": cls.normalize_dsl(dsl),
            "updated_at": int(time.time()),
        }


    @classmethod
    def create_if_absent(
        cls,
        canvas_id: str,
        tenant_id: str,
        runtime_user_id: str,
        dsl,
        canvas_category=CanvasCategory.Agent,
        title="",
    ):
        """Create a runtime replica if it does not exist; otherwise keep existing state."""
        replica_key = cls._replica_key(canvas_id, str(tenant_id), str(runtime_user_id))
        payload = cls._read_payload(replica_key)
        if payload:
            return payload
        payload = cls._build_payload(canvas_id, str(tenant_id), str(runtime_user_id), dsl, canvas_category, title)
        cls._write_payload(replica_key, payload)
        return payload


    @classmethod
    def bootstrap(
        cls,
        canvas_id: str,
        tenant_id: str,
        runtime_user_id: str,
        dsl,
        canvas_category=CanvasCategory.Agent,
        title="",
    ):
        """Bootstrap replica by creating it when absent and keeping existing runtime state."""
        return cls.create_if_absent(
            canvas_id=canvas_id,
            tenant_id=tenant_id,
            runtime_user_id=runtime_user_id,
            dsl=dsl,
            canvas_category=canvas_category,
            title=title,
        )


    @classmethod
    def load_for_run(cls, canvas_id: str, tenant_id: str, runtime_user_id: str):
        """Load current runtime replica used by /completion."""
        replica_key = cls._replica_key(canvas_id, str(tenant_id), str(runtime_user_id))
        return cls._read_payload(replica_key)


    @classmethod
    def replace_for_set(
        cls,
        canvas_id: str,
        tenant_id: str,
        runtime_user_id: str,
        dsl,
        canvas_category=CanvasCategory.Agent,
        title="",
    ):
        """Replace replica content for `/set` under lock."""
        replica_key = cls._replica_key(canvas_id, str(tenant_id), str(runtime_user_id))
        lock_key = cls._lock_key(canvas_id, str(tenant_id), str(runtime_user_id))
        lock = cls._acquire_lock_with_retry(lock_key)
        if not lock:
            logging.error("Failed to acquire canvas replica lock after retry: %s", lock_key)
            return False

        try:
            updated_payload = cls._build_payload(
                canvas_id=canvas_id,
                tenant_id=str(tenant_id),
                runtime_user_id=str(runtime_user_id),
                dsl=dsl,
                canvas_category=canvas_category,
                title=title,
            )
            cls._write_payload(replica_key, updated_payload)
            return True
        except Exception:
            logging.exception("Failed to replace canvas replica from /set.")
            return False
        finally:
            try:
                lock.release()
            except Exception:
                logging.exception("Failed to release canvas replica lock: %s", lock_key)


    @classmethod
    def _acquire_lock_with_retry(cls, lock_key: str):
        """Acquire distributed lock with bounded retries; return lock object or None."""
        lock = RedisDistributedLock(
            lock_key,
            timeout=cls.LOCK_TIMEOUT_SECS,
            blocking_timeout=cls.LOCK_BLOCKING_TIMEOUT_SECS,
        )
        for idx in range(cls.LOCK_RETRY_ATTEMPTS):
            if lock.acquire():
                return lock
            if idx < cls.LOCK_RETRY_ATTEMPTS - 1:
                time.sleep(cls.LOCK_RETRY_SLEEP_SECS + random.uniform(0, 0.1))
        return None


    @classmethod
    def commit_after_run(
        cls,
        canvas_id: str,
        tenant_id: str,
        runtime_user_id: str,
        dsl,
        canvas_category=CanvasCategory.Agent,
        title="",
    ):
        """
        Commit post-run DSL into replica.

        Returns:
            bool: True on committed/saved, False on commit failure.
        """
        new_dsl = cls.normalize_dsl(dsl)
        replica_key = cls._replica_key(canvas_id, str(tenant_id), str(runtime_user_id))

        try:
            latest_payload = cls._read_payload(replica_key)

            # Always write latest runtime DSL back to Redis first.
            updated_payload = cls._build_payload(
                canvas_id=canvas_id,
                tenant_id=str(tenant_id),
                runtime_user_id=str(runtime_user_id),
                dsl=new_dsl,
                canvas_category=canvas_category if not latest_payload else (canvas_category or latest_payload.get("canvas_category", CanvasCategory.Agent)),
                title=title if not latest_payload else (title or latest_payload.get("title", "")),
            )
            cls._write_payload(replica_key, updated_payload)

            return True
        except Exception:
            logging.exception("Failed to commit canvas runtime replica.")
            return False
