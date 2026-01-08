"""GraphState for LangGraph workflow execution.

This module provides state management compatible with RAGFlow's variable system.
"""

from typing import Any, Dict, Optional
from pydantic import BaseModel, Field


class GraphState(BaseModel):
    """LangGraph state management, compatible with RAGFlow's variable system."""

    # Global variable pool (compatible with globals)
    globals: Dict[str, Any] = Field(default_factory=dict)

    # Component output pool (compatible with component outputs)
    variables_pool: Dict[str, Dict[str, Any]] = Field(default_factory=dict)

    # Message history (compatible with history)
    messages: list[tuple[str, Any]] = Field(default_factory=list)

    # Retrieval results (compatible with retrieval)
    retrieval: list[Dict[str, Any]] = Field(default_factory=list)

    # Current executing node ID
    current_node_id: Optional[str] = None

    # Metadata
    tenant_id: Optional[str] = None
    task_id: Optional[str] = None
    canvas_id: Optional[str] = None

    def get_variable_value(self, exp: str) -> Any:
        """
        Resolve variable reference from expression.

        Supports formats:
        - {component_id@variable.key} - Component variable reference
        - sys.variable - Global variable reference

        Args:
            exp: Variable expression string

        Returns:
            Resolved value or None if not found
        """
        import re

        # Match pattern: {component_id@variable} or {sys.variable}
        pattern = re.compile(r"\{([a-zA-Z0-9_]+@[A-Za-z0-9_.-]+|sys\.[A-Za-z0-9_.]+)\}")
        match = pattern.match(exp)

        if not match:
            # If no curly braces, return as-is
            return exp

        exp = match.group(1)

        # Handle sys.* variables
        if exp.startswith("sys."):
            var_name = exp[4:]
            return self.globals.get(var_name)

        # Handle component@variable references
        if "@" in exp:
            cpn_id, var_nm = exp.split("@", 1)
            return self.get_component_variable(cpn_id, var_nm)

        return None

    def get_component_variable(self, node_id: str, key: str) -> Any:
        """
        Get component output variable.

        Args:
            node_id: Component ID
            key: Variable key (supports nested access like output.field.subfield)

        Returns:
            Variable value or None if not found
        """
        if node_id not in self.variables_pool:
            return None

        parts = key.split(".", 1)
        root_key = parts[0]
        root_val = self.variables_pool[node_id].get(root_key)

        if len(parts) == 1:
            return root_val

        # Support nested access: obj.sub_field
        return self._get_nested_value(root_val, parts[1])

    def set_component_variable(self, node_id: str, key: str, value: Any):
        """
        Set component output variable.

        Args:
            node_id: Component ID
            key: Variable key
            value: Variable value
        """
        if node_id not in self.variables_pool:
            self.variables_pool[node_id] = {}
        self.variables_pool[node_id][key] = value

    def _get_nested_value(self, obj: Any, path: str) -> Any:
        """
        Get nested object value by dot-separated path.

        Args:
            obj: Object to traverse
            path: Dot-separated path (e.g., "field.subfield")

        Returns:
            Nested value or None if not found
        """
        if obj is None:
            return None

        for key in path.split('.'):
            if isinstance(obj, dict):
                obj = obj.get(key)
            elif isinstance(obj, list):
                try:
                    obj = obj[int(key)]
                except (ValueError, IndexError):
                    return None
            else:
                return None
        return obj

    def model_post_init(self, __context: Any) -> None:
        """
        Initialize default values after model creation.
        """
        super().model_post_init(__context)
        # Ensure globals has system variables if not provided
        if "sys.query" not in self.globals:
            self.globals["sys.query"] = ""
        if "sys.user_id" not in self.globals:
            self.globals["sys.user_id"] = self.tenant_id or ""
        if "sys.conversation_turns" not in self.globals:
            self.globals["sys.conversation_turns"] = 0
        if "sys.files" not in self.globals:
            self.globals["sys.files"] = []
