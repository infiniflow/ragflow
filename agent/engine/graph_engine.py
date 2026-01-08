"""GraphEngine - LangGraph-based workflow execution engine."""

import logging
from typing import Dict, Any, Optional

from langgraph.checkpoint.memory import MemorySaver
from langgraph.constants import END, START
from langgraph.graph import StateGraph

from agent.engine.checkpoint import RedisCheckpoint
from agent.engine.graph_state import GraphState
from agent.callback.sse_callback import SSECallback
from agent.nodes.component_node import ComponentNode

logger = logging.getLogger(__name__)


class GraphEngine:
    """
    LangGraph-based workflow execution engine.

    This engine builds and executes workflows using LangGraph, directly
    parsing the existing DSL format without any conversion.
    """

    def __init__(
        self,
        dsl: dict,
        tenant_id: str,
        task_id: str,
        canvas_id: str = None,
        globals: dict = None
    ):
        """
        Initialize GraphEngine.

        Args:
            dsl: DSL dictionary (existing format, no conversion needed)
            tenant_id: Tenant ID for namespace isolation
            task_id: Task ID for execution tracking
            canvas_id: Canvas ID for context
            globals: Global variables dictionary
        """
        self.dsl = dsl  # Use existing DSL format directly
        self._tenant_id = tenant_id
        self.task_id = task_id
        self.canvas_id = canvas_id
        self.globals = globals or {}

        # Initialize GraphState
        self.graph_state = GraphState(
            tenant_id=tenant_id,
            task_id=task_id,
            canvas_id=canvas_id,
            globals=globals,
            history=dsl.get("history", []),
            retrieval=dsl.get("retrieval", {"chunks": [], "doc_aggs": []})
        )

        # Initialize callback
        self.callback = SSECallback(task_id=task_id)

        # Node management
        self.nodes_map: Dict[str, ComponentNode] = {}

        # LangGraph builder
        self.graph_builder = StateGraph(self.graph_state)
        self.graph = None

        # Redis Checkpoint
        self.checkpoint = RedisCheckpoint(
            tenant_id=tenant_id,
            task_id=task_id,
            canvas_id=canvas_id
        )

        # Build the graph
        self._build_graph()

    def _build_graph(self):
        """
        Build LangGraph directly from DSL components.

        This method parses the existing DSL structure to create nodes
        and edges without any format conversion.
        """
        components = self.dsl.get("components", {})

        # Step 1: Create nodes from components
        for cpn_id, cpn_data in components.items():
            component_obj = cpn_data.get("obj", {})
            component_name = component_obj.get("component_name", "Unknown")
            params = component_obj.get("params", {})

            # Determine node type
            node_type = self._map_component_to_node_type(component_name)

            node = ComponentNode(
                node_id=cpn_id,
                node_type=node_type,
                component_name=component_name,
                params=params,
                graph_state=self.graph_state,
                callback=self.callback
            )

            self.nodes_map[cpn_id] = node
            self.graph_builder.add_node(cpn_id, node.run)

        # Step 2: Create edges from downstream/upstream
        for cpn_id, cpn_data in components.items():
            downstream = cpn_data.get("downstream", [])

            for target_id in downstream:
                source_node = self.nodes_map.get(cpn_id)
                target_node = self.nodes_map.get(target_id)

                if not source_node or not target_node:
                    logger.warning(f"Skipping edge {cpn_id} -> {target_id}: node not found")
                    continue

                if source_node.type in ["switch", "categorize"]:
                    # Conditional edge
                    self.graph_builder.add_conditional_edges(
                        cpn_id,
                        source_node.route_node,
                        {target_id: target_id}
                    )
                else:
                    # Regular edge
                    self.graph_builder.add_edge(cpn_id, target_id)

        # Step 3: Set start and end nodes
        start_nodes = [n for n in self.nodes_map.values() if n.type == "start"]
        end_nodes = [n for n in self.nodes_map.values() if n.type == "end"]

        if start_nodes:
            self.graph_builder.add_edge(START, start_nodes[0].id)
        for end_node in end_nodes:
            self.graph_builder.add_edge(end_node.id, END)

        # Step 4: Compile graph with Redis Checkpoint
        try:
            self.graph = self.graph_builder.compile(
                checkpointer=self.checkpoint
            )
        except Exception as e:
            logger.warning(f"Failed to compile with Redis checkpoint, using MemorySaver: {e}")
            self.graph = self.graph_builder.compile(checkpointer=MemorySaver())

    def _map_component_to_node_type(self, component_name: str) -> str:
        """
        Map component name to node type.

        Args:
            component_name: Component class name

        Returns:
            Node type string
        """
        mapping = {
            "Begin": "start",
            "End": "end",
            "Message": "message",
            "LLM": "llm",
            "Retrieval": "retrieval",
            "Generate": "generate",
            "Switch": "switch",
            "Categorize": "categorize",
            "Iteration": "iteration",
            "Loop": "loop",
            "UserFillUp": "user_input",
            "Answer": "answer",
            "Note": "note",
        }
        return mapping.get(component_name, "generic")

    async def run(self, **kwargs) -> Any:
        """
        Execute workflow and stream events.

        Args:
            **kwargs: Workflow input parameters

        Yields:
            Event strings in SSE format
        """
        self.callback.on_workflow_started(kwargs)

        config = {
            "configurable": {"thread_id": self.task_id},
            "recursion_limit": 100
        }

        try:
            # Prepare initial state
            initial_state = {
                "globals": self.globals
            }

            # Stream execution
            async for state_update in self.graph.astream(
                initial_state,
                config=config
            ):
                # Process GraphState updates
                async for event in self.callback.process_state_update(state_update):
                    yield event

        except Exception as e:
            logger.error(f"Workflow execution failed: {e}")
            self.callback.on_error(e)
            yield self.callback.format_error(e)

        self.callback.on_workflow_finished()

    def get_variable_value(self, exp: str) -> Any:
        """
        Get variable value (compatible with variable references).

        Args:
            exp: Variable expression

        Returns:
            Resolved value
        """
        return self.graph_state.get_variable_value(exp)

    def get_node(self, node_id: str) -> Optional[ComponentNode]:
        """
        Get node by ID.

        Args:
            node_id: Node ID

        Returns:
            ComponentNode or None
        """
        return self.nodes_map.get(node_id)

    def get_state(self) -> GraphState:
        """
        Get current GraphState.

        Returns:
            Current GraphState
        """
        return self.graph_state
