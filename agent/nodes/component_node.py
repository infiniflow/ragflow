"""ComponentNode adapter for LangGraph node execution."""

import logging
from typing import Any, Dict

from agent.component import ComponentBase, component_class
from agent.engine.graph_state import GraphState
from agent.callback.sse_callback import SSECallback

logger = logging.getLogger(__name__)


class ComponentNode:
    """
    RAGFlow Component to LangGraph Node adapter.

    This class adapts RAGFlow's ComponentBase to work as a LangGraph node.
    It creates component instances from DSL and executes them within the
    LangGraph workflow context.
    """

    def __init__(
        self,
        node_id: str,
        node_type: str,
        component_name: str,
        params: Dict,
        graph_state: GraphState,
        callback: SSECallback
    ):
        """
        Initialize ComponentNode.

        Args:
            node_id: Unique node ID
            node_type: Node type (start, end, llm, message, etc.)
            component_name: Component class name
            params: Component parameters from DSL
            graph_state: Shared GraphState instance
            callback: SSE callback for event streaming
        """
        self.id = node_id
        self.type = node_type
        self.name = component_name
        self.params = params
        self.graph_state = graph_state
        self.callback = callback

        # Create Component instance
        self.component = self._create_component()

    def _create_component(self) -> ComponentBase:
        """
        Create component instance from parameters.

        Returns:
            Initialized ComponentBase instance
        """
        try:
            # Create parameter object
            param_class = component_class(self.name + "Param")
            param = param_class()
            param.update(self.params)

            # Validate parameters
            try:
                param.check()
            except Exception as e:
                logger.error(f"Parameter validation failed for {self.name}: {e}")
                raise ValueError(f"{self.name}: {e}")

            # Create a virtual canvas (only for component initialization)
            # We avoid circular import by lazy loading
            from agent.canvas import Canvas as VirtualCanvas
            virtual_canvas = VirtualCanvas.__new__(VirtualCanvas)
            virtual_canvas.globals = self.graph_state.globals

            # Create component instance
            component_instance = component_class(self.name)(
                virtual_canvas,
                self.id,
                param
            )

            return component_instance

        except Exception as e:
            logger.error(f"Failed to create component {self.name}: {e}")
            raise

    async def run(self, state: GraphState) -> GraphState:
        """
        LangGraph node execution function.

        This method is called by LangGraph when the node is executed.
        It triggers component execution and updates the GraphState.

        Args:
            state: Current GraphState

        Returns:
            Updated GraphState

        Raises:
            Exception: If component execution fails
        """
        state.current_node_id = self.id
        start_time = __import__('time').time()

        # Trigger node_started event
        self.callback.on_node_started(
            node_id=self.id,
            node_name=self.name,
            component_type=getattr(self.component, 'component_name', self.name),
            inputs=self._resolve_inputs()
        )

        # Execute component
        try:
            result = None
            if hasattr(self.component, "invoke_async"):
                result = await self.component.invoke_async()
            else:
                result = self.component.invoke()

            # Save component output to GraphState
            state.variables_pool[self.id] = result

            # Handle special component types
            if self.type == "message":
                self._handle_message(result, state)
            elif self.type in ["switch", "categorize"]:
                return self._handle_conditional(state, result)

            # Trigger node_finished event
            elapsed_time = __import__('time').time() - start_time
            self.callback.on_node_finished(
                node_id=self.id,
                node_name=self.name,
                outputs=result,
                error=None,
                elapsed_time=elapsed_time
            )

        except Exception as e:
            logger.error(f"Component {self.name} execution failed: {e}")
            state.variables_pool[self.id] = {"_ERROR": str(e)}

            elapsed_time = __import__('time').time() - start_time
            self.callback.on_node_finished(
                node_id=self.id,
                node_name=self.name,
                outputs=None,
                error=str(e),
                elapsed_time=elapsed_time
            )
            raise

        return state

    def _resolve_inputs(self) -> Dict[str, Any]:
        """
        Resolve component inputs, supporting variable references.

        Returns:
            Dictionary of resolved input values
        """
        inputs = {}

        # Try to get input definitions from component parameter
        try:
            param_inputs = self.component._param.inputs
        except AttributeError:
            param_inputs = {}

        for key, input_def in param_inputs.items():
            value = input_def.get("value")

            # Check if value is a variable reference
            if isinstance(value, str) and "{" in value and "}" in value:
                resolved_value = self.graph_state.get_variable_value(value)
                self.component.set_input_value(key, resolved_value)
                inputs[key] = resolved_value
            else:
                inputs[key] = value

        return inputs

    def _handle_message(self, result: Dict, state: GraphState) -> None:
        """
        Handle Message component streaming output.

        Args:
            result: Component execution result
            state: Current GraphState
        """
        content = result.get("content") if result else None

        if content:
            # Support streaming output
            if hasattr(content, "__call__"):  # partial / generator
                for chunk in content():
                    self.callback.on_message({"content": chunk})
            else:
                self.callback.on_message({"content": content})

        # Save to message history
        state.messages.append(("assistant", result))

    def _handle_conditional(self, state: GraphState, result: Dict) -> GraphState:
        """
        Handle conditional nodes (Switch/Categorize).

        Args:
            state: Current GraphState
            result: Component execution result

        Returns:
            Updated GraphState with routing information
        """
        # Result contains next node ID
        next_node_id = result.get("_next")

        if next_node_id:
            # LangGraph will select edge based on route_node return value
            state.current_node_id = next_node_id

        return state

    def route_node(self, state: GraphState) -> str:
        """
        Routing function for conditional nodes.

        This method is called by LangGraph to determine which edge to follow
        from a conditional node.

        Args:
            state: Current GraphState

        Returns:
            Next node ID or END
        """
        output = state.variables_pool.get(self.id, {})
        next_node_id = output.get("_next")

        return next_node_id or "END"

    def __str__(self) -> str:
        """
        Return string representation of component for DSL serialization.

        Returns:
            JSON string representation
        """
        try:
            return self.component.json()
        except Exception:
            # Fallback if json() method not available
            import json
            return json.dumps({
                "component_name": self.name,
                "params": self.params
            })
