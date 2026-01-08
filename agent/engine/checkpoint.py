"""Redis Checkpoint implementation for LangGraph state persistence."""

import json
import logging
from typing import Optional, TypedDict, Any
from langchain_core.runnables import RunnableConfig
from langgraph.checkpoint.base import BaseCheckpointSaver
from agent.engine.graph_state import GraphState

logger = logging.getLogger(__name__)

# Checkpoint data structure
class Checkpoint(TypedDict):
    """Checkpoint data structure."""
    id: str
    thread_id: str
    state: dict
    metadata: dict
    step: int


class RedisCheckpoint(BaseCheckpointSaver):
    """
    Redis Checkpoint implementation for persisting GraphState.

    This class provides checkpoint persistence using Redis, enabling
    workflow state recovery and resumption.
    """

    def __init__(
        self,
        tenant_id: str,
        task_id: str,
        canvas_id: str = None,
        ttl: int = 3600  # 1 hour default
    ):
        """
        Initialize Redis Checkpoint.

        Args:
            tenant_id: Tenant ID for namespace isolation
            task_id: Task ID for checkpoint identification
            canvas_id: Canvas ID for additional context
            ttl: Time to live for checkpoints in seconds
        """
        self.tenant_id = tenant_id
        self.task_id = task_id
        self.canvas_id = canvas_id
        self.ttl = ttl
        self._key_prefix = f"checkpoint:{tenant_id}:{canvas_id}:{task_id}"

        # Import REDIS_CONN lazily to avoid import issues
        try:
            from rag.utils.redis_conn import REDIS_CONN
            self.redis = REDIS_CONN
        except ImportError:
            logger.warning("REDIS_CONN not available, using None")
            self.redis = None

    def _get_checkpoint_key(self, thread_id: str) -> str:
        """Get checkpoint Redis key."""
        return f"{self._key_prefix}:{thread_id}"

    def _get_state_key(self, thread_id: str, checkpoint_id: str) -> str:
        """Get state Redis key."""
        return f"{self._key_prefix}:{thread_id}:{checkpoint_id}:state"

    def get(self, config: RunnableConfig) -> Optional[Checkpoint]:
        """
        Get checkpoint by configuration.

        Args:
            config: RunnableConfig containing thread_id

        Returns:
            Checkpoint object or None if not found
        """
        if self.redis is None:
            return None

        thread_id = config.get("configurable", {}).get("thread_id")
        if not thread_id:
            return None

        key = self._get_checkpoint_key(thread_id)
        try:
            data = self.redis.get(key)
            if not data:
                return None

            checkpoint_data = json.loads(data)
            return Checkpoint(**checkpoint_data)
        except Exception as e:
            logger.error(f"Failed to get checkpoint: {e}")
            return None

    def put(self, config: RunnableConfig, checkpoint: Checkpoint) -> Checkpoint:
        """
        Save checkpoint to Redis.

        Args:
            config: RunnableConfig containing thread_id
            checkpoint: Checkpoint object to save

        Returns:
            Saved checkpoint
        """
        if self.redis is None:
            return checkpoint

        thread_id = config.get("configurable", {}).get("thread_id")
        checkpoint_key = self._get_checkpoint_key(thread_id)
        state_key = self._get_state_key(thread_id, checkpoint.id)

        try:
            # Save checkpoint metadata
            self.redis.set(
                checkpoint_key,
                json.dumps({
                    "id": checkpoint.id,
                    "thread_id": thread_id,
                    "metadata": checkpoint.metadata,
                    "step": checkpoint.step
                }),
                ex=self.ttl
            )

            # Save full state
            state_data = {
                "globals": checkpoint.state.get("globals", {}),
                "variables_pool": checkpoint.state.get("variables_pool", {}),
                "messages": checkpoint.state.get("messages", []),
                "retrieval": checkpoint.state.get("retrieval", []),
                "current_node_id": checkpoint.state.get("current_node_id")
            }

            self.redis.set(
                state_key,
                json.dumps(state_data),
                ex=self.ttl
            )

        except Exception as e:
            logger.error(f"Failed to save checkpoint: {e}")

        return checkpoint

    def list(self, config: RunnableConfig, limit: int = 10, before: Optional[str] = None) -> list[Checkpoint]:
        """
        List historical checkpoints.

        Args:
            config: RunnableConfig containing thread_id
            limit: Maximum number of checkpoints to return
            before: Optional checkpoint ID to list before

        Returns:
            List of checkpoints sorted by step (descending)
        """
        if self.redis is None:
            return []

        thread_id = config.get("configurable", {}).get("thread_id")
        if not thread_id:
            return []

        checkpoints = []

        try:
            # Use Redis SCAN to iterate through all checkpoints for this thread
            pattern = f"{self._key_prefix}:{thread_id}:*"
            for key in self.redis.scan_iter(match=pattern):
                key_str = key.decode() if isinstance(key, bytes) else key

                # Skip state keys
                if key_str.endswith(":state"):
                    continue

                data = self.redis.get(key)
                if data:
                    checkpoint_data = json.loads(data)
                    checkpoints.append(Checkpoint(**checkpoint_data))

            # Sort by step descending
            checkpoints.sort(key=lambda x: x.step, reverse=True)

            # Filter by 'before' if specified
            if before:
                checkpoints = [c for c in checkpoints if c.metadata.get("parent_id") == before or c.id == before]

            return checkpoints[:limit]

        except Exception as e:
            logger.error(f"Failed to list checkpoints: {e}")
            return []

    def delete_checkpoint(self, checkpoint_id: str) -> None:
        """
        Delete specified checkpoint.

        Args:
            checkpoint_id: Checkpoint ID to delete
        """
        if self.redis is None:
            return

        try:
            # Find all checkpoints with this ID (across all threads)
            pattern = f"{self._key_prefix}:*"
            for key in self.redis.scan_iter(match=pattern):
                key_str = key.decode() if isinstance(key, bytes) else key

                data = self.redis.get(key)
                if data:
                    checkpoint_data = json.loads(data)
                    if checkpoint_data.get("id") == checkpoint_id:
                        # Delete checkpoint metadata
                        self.redis.delete(key)

                        # Delete associated state
                        thread_id = checkpoint_data.get("thread_id")
                        state_key = self._get_state_key(thread_id, checkpoint_id)
                        self.redis.delete(state_key)
                        break

        except Exception as e:
            logger.error(f"Failed to delete checkpoint: {e}")

    def clear(self) -> None:
        """Clear all checkpoints for this tenant/canvas/task."""
        if self.redis is None:
            return

        try:
            pattern = f"{self._key_prefix}:*"
            for key in self.redis.scan_iter(match=pattern):
                self.redis.delete(key)

        except Exception as e:
            logger.error(f"Failed to clear checkpoints: {e}")

    # Required BaseCheckpointSaver methods (minimal implementation)
    def get_tuple(self, config: RunnableConfig) -> tuple[Optional[Checkpoint], Optional[dict]]:
        """Get checkpoint tuple (for LangGraph compatibility)."""
        checkpoint = self.get(config)
        return (checkpoint, checkpoint.get("metadata") if checkpoint else None)

    def put_tuple(self, config: RunnableConfig, checkpoint: tuple[Checkpoint, dict]) -> tuple[Checkpoint, dict]:
        """Put checkpoint tuple (for LangGraph compatibility)."""
        self.put(config, checkpoint[0])
        return checkpoint
