"""SSE Callback for streaming event responses."""

import json
import time
import logging
from typing import Any, Dict, Generator

logger = logging.getLogger(__name__)


class SSECallback:
    """
    Server-Sent Events callback for streaming workflow execution events.

    This callback maintains compatibility with existing UI event formats.
    """

    def __init__(self, task_id: str):
        """
        Initialize SSE callback.

        Args:
            task_id: Task ID for event correlation
        """
        self.task_id = task_id
        self.message_id = None
        self.created_at = int(time.time())
        self._events = []

    def format_event(self, event_type: str, data: Dict) -> Dict[str, Any]:
        """
        Format event data structure.

        Args:
            event_type: Event type (e.g., "workflow_started", "node_started")
            data: Event data payload

        Returns:
            Formatted event dictionary
        """
        event = {
            "event": event_type,
            "message_id": self.message_id,
            "created_at": self.created_at,
            "task_id": self.task_id,
            "data": data
        }
        return event

    def on_workflow_started(self, inputs: Dict[str, Any]):
        """
        Trigger workflow started event.

        Args:
            inputs: Workflow input parameters
        """
        self._events.append(self.format_event(
            "workflow_started",
            {"inputs": inputs}
        ))

    def on_node_started(
        self,
        node_id: str,
        node_name: str,
        component_type: str,
        inputs: Dict[str, Any]
    ):
        """
        Trigger node started event.

        Args:
            node_id: Node/component ID
            node_name: Display name
            component_type: Component type
            inputs: Node input parameters
        """
        self._events.append(self.format_event(
            "node_started",
            {
                "component_id": node_id,
                "component_name": node_name,
                "component_type": component_type,
                "inputs": inputs,
                "created_at": int(time.time())
            }
        ))

    def on_node_finished(
        self,
        node_id: str,
        node_name: str,
        outputs: Dict[str, Any],
        error: str = None,
        elapsed_time: float = 0
    ):
        """
        Trigger node finished event.

        Args:
            node_id: Node/component ID
            node_name: Display name
            outputs: Node output results
            error: Error message if execution failed
            elapsed_time: Execution time in seconds
        """
        self._events.append(self.format_event(
            "node_finished",
            {
                "component_id": node_id,
                "component_name": node_name,
                "outputs": outputs,
                "error": error,
                "elapsed_time": elapsed_time,
                "created_at": int(time.time())
            }
        ))

    def on_message(self, data: Dict[str, Any]):
        """
        Trigger message output event.

        Args:
            data: Message data payload
        """
        self._events.append(self.format_event("message", data))

    def on_error(self, error: Exception):
        """
        Trigger error event.

        Args:
            error: Exception object
        """
        self._events.append(self.format_event(
            "error",
            {"message": str(error)}
        ))

    def on_workflow_finished(self, status: str = "completed"):
        """
        Trigger workflow finished event.

        Args:
            status: Workflow completion status
        """
        self._events.append(self.format_event(
            "workflow_finished",
            {"status": status}
        ))

    def format_error(self, error: Exception) -> str:
        """
        Format error as SSE message string.

        Args:
            error: Exception object

        Returns:
            Formatted SSE error string
        """
        event = {
            "code": 500,
            "message": str(error),
            "data": False
        }
        return f"data:{json.dumps(event, ensure_ascii=False)}\n\n"

    def process_state_update(self, state_update: Any) -> Generator[str, None, None]:
        """
        Process GraphState update and yield event stream.

        Args:
            state_update: Updated state from GraphEngine

        Yields:
            Formatted SSE event strings
        """
        for event in self._events:
            yield f"data:{json.dumps(event, ensure_ascii=False)}\n\n"

        self._events.clear()

    def get_events(self) -> list[Dict[str, Any]]:
        """
        Get all pending events.

        Returns:
            List of event dictionaries
        """
        return self._events.copy()

    def clear_events(self):
        """Clear all pending events."""
        self._events.clear()
