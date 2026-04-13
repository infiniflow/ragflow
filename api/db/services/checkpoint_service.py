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
# CHECKPOINT: Service for managing task checkpoint state, enabling resume after crash/interruption.
# Uses Redis SADD for atomic, concurrent-safe checkpoint tracking, with DB as durable fallback.

import logging

from rag.utils.redis_conn import REDIS_CONN


CHECKPOINT_KEY_PREFIX = "task_checkpoint:"
CHECKPOINT_TTL = 60 * 60 * 24 * 7  # 7 days


def _checkpoint_key(task_id: str) -> str:
    return f"{CHECKPOINT_KEY_PREFIX}{task_id}"


def save_checkpoint(task_id: str, completed_doc_id: str) -> None:
    """Record a completed doc_id for the given task (atomic, concurrent-safe).

    Args:
        task_id: The task ID to checkpoint.
        completed_doc_id: The document ID that just finished processing.
    """
    try:
        key = _checkpoint_key(task_id)
        REDIS_CONN.REDIS.sadd(key, completed_doc_id)
        REDIS_CONN.REDIS.expire(key, CHECKPOINT_TTL)
    except Exception:
        logging.exception(f"save_checkpoint failed for task {task_id}, doc {completed_doc_id}")


def load_checkpoint(task_id: str) -> set:
    """Load the set of completed doc_ids for a task.

    Args:
        task_id: The task ID to look up.

    Returns:
        A set of doc_id strings that have already been processed, or empty set.
    """
    try:
        key = _checkpoint_key(task_id)
        members = REDIS_CONN.REDIS.smembers(key)
        if members:
            return set(members)
        return set()
    except Exception:
        logging.exception(f"load_checkpoint failed for task {task_id}")
        return set()


def clear_checkpoint(task_id: str) -> None:
    """Clear checkpoint data after a task completes successfully.

    Args:
        task_id: The task ID whose checkpoint should be cleared.
    """
    try:
        REDIS_CONN.REDIS.delete(_checkpoint_key(task_id))
    except Exception:
        logging.exception(f"clear_checkpoint failed for task {task_id}")
