"""Redis Checkpoint implementation for LangGraph state persistence."""

import json
import logging
from typing import Optional, TypedDict
from langchain_core.runnables import RunnableConfig
from langgraph.checkpoint.base import BaseCheckpointSaver
from agent.engine.graph_state import GraphState

logger = logging.getLogger(__name__)


# Internal checkpoint data structure for Redis storage
class _StoredCheckpoint(TypedDict):
    """Internal checkpoint structure for Redis persistence."""
    id: str
    thread_id: str
    state_dict: dict  # GraphState serialized as dict
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

    def _get_state_key(self, thread_id: str, checkpoint_id: str = "") -> str:
        """
        Get state Redis key.

        Args:
            thread_id: Thread ID
            checkpoint_id: Checkpoint ID (optional, for specific checkpoint state)

        Returns:
            Redis key for state storage
        """
        # Always use a consistent state key for the thread
        # This simplifies state management and retrieval
        return f"{self._key_prefix}:{thread_id}:state"

    def get(self, config: RunnableConfig) -> Optional[dict]:
        """
        Get checkpoint by configuration and reconstruct GraphState.

        Returns checkpoint data with GraphState as dict (for LangGraph compatibility).

        Args:
            config: RunnableConfig containing thread_id

        Returns:
            Checkpoint dict or None if not found
        """
        if self.redis is None:
            return None

        thread_id = config.get("configurable", {}).get("thread_id")
        if not thread_id:
            return None

        key = self._get_checkpoint_key(thread_id)
        state_key = self._get_state_key(thread_id, "")

        try:
            # Get checkpoint metadata
            data = self.redis.get(key)
            if not data:
                return None

            checkpoint_data = json.loads(data)

            # Get state data and reconstruct GraphState
            state_data = self.redis.get(state_key)
            if state_data:
                state_dict = json.loads(state_data)
                # Reconstruct GraphState from dict to ensure validity
                graph_state = GraphState(**state_dict)
                # Return GraphState as dict for LangGraph (channel_values)
                checkpoint_data["channel_values"] = graph_state.model_dump()
            else:
                # If no state data, create empty GraphState
                checkpoint_data["channel_values"] = GraphState().model_dump()

            return checkpoint_data
        except Exception as e:
            logger.error(f"Failed to get checkpoint: {e}")
            return None

    def put(self, config: RunnableConfig, checkpoint: dict) -> dict:
        """
        Save checkpoint to Redis.

        Args:
            config: RunnableConfig containing thread_id
            checkpoint: Checkpoint dict with GraphState in channel_values

        Returns:
            Saved checkpoint dict
        """
        if self.redis is None:
            return checkpoint

        thread_id = config.get("configurable", {}).get("thread_id")
        checkpoint_key = self._get_checkpoint_key(thread_id)
        state_key = self._get_state_key(thread_id, checkpoint.get("id", ""))

        try:
            # Save checkpoint metadata
            self.redis.set(
                checkpoint_key,
                json.dumps({
                    "id": checkpoint.get("id"),
                    "thread_id": thread_id,
                    "metadata": checkpoint.get("metadata"),
                    "step": checkpoint.get("step", 0)
                }),
                exp=self.ttl
            )

            # Extract GraphState from channel_values and save
            channel_values = checkpoint.get("channel_values", {})

            # Validate and convert to GraphState for serialization
            graph_state = GraphState(**channel_values)
            state_data = graph_state.model_dump()

            self.redis.set(
                state_key,
                json.dumps(state_data),
                exp=self.ttl
            )

        except Exception as e:
            logger.error(f"Failed to save checkpoint: {e}")

        return checkpoint

    def list(self, config: RunnableConfig, limit: int = 10, before: Optional[str] = None) -> list[dict]:
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
                # Skip state keys
                if isinstance(key, bytes):
                    key = key.decode()
                if key.endswith(":state"):
                    continue

                data = self.redis.get(key)
                if data:
                    checkpoint_data = json.loads(data)
                    checkpoints.append(checkpoint_data)

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
    def get_tuple(self, config: RunnableConfig) -> tuple[Optional[dict], Optional[dict]]:
        """Get checkpoint tuple (for LangGraph compatibility)."""
        checkpoint = self.get(config)
        return (checkpoint, checkpoint.get("metadata") if checkpoint else None)

    def put_tuple(self, config: RunnableConfig, checkpoint: tuple[dict, dict]) -> tuple[dict, dict]:
        """Put checkpoint tuple (for LangGraph compatibility)."""
        self.put(config, checkpoint[0])
        return checkpoint
