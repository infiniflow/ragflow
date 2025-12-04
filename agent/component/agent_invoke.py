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
"""
AgentInvoke Component

This component enables agent-to-agent invocation within RAGFlow, allowing a portal
agent to dynamically delegate tasks to specialized downstream agents/workflows.

Key Features:
- Dynamic routing: Route user requests to specialized agents based on intent
- Parameter passing: Pass inputs and context to downstream agents
- Result aggregation: Collect and return agent outputs
- Modular design: Agents can be developed and maintained independently

Use Case Example:
    Portal Agent receives: "Analyze Q3 sales data"
    -> Invokes Sales Analysis Agent with parameters: {"period": "Q3", ...}
    -> Sales Analysis Agent processes and returns results
    -> Portal Agent formats and returns to user
"""

import asyncio
import json
import logging
import os
import time
from abc import ABC

from agent.component.base import ComponentBase, ComponentParamBase
from common.connection_utils import timeout


class AgentInvokeParam(ComponentParamBase):
    """
    Parameters for the AgentInvoke component.
    
    Attributes:
        agent_id (str): The ID of the agent/workflow to invoke
        agent_name (str): Optional human-readable name for logging
        inputs (dict): Input parameters to pass to the agent
        query (str): Optional query/question to pass to the agent
        timeout_seconds (int): Maximum time to wait for agent response
        create_new_session (bool): Whether to create a new session for each invocation
    """
    
    def __init__(self):
        super().__init__()
        self.agent_id = ""
        self.agent_name = ""
        self.inputs = {}
        self.query = ""
        self.timeout_seconds = 300  # 5 minutes default
        self.create_new_session = True
        self.session_id = None
    
    def check(self):
        """Validate component parameters"""
        self.check_empty(self.agent_id, "Agent ID")
        self.check_positive_integer(self.timeout_seconds, "Timeout in seconds")
        self.check_boolean(self.create_new_session, "Create new session")


class AgentInvoke(ComponentBase, ABC):
    """
    Component for invoking downstream agents/workflows.
    
    This component allows a portal agent to delegate tasks to specialized
    agents, enabling a modular, maintainable AI portal architecture.
    """
    
    component_name = "AgentInvoke"
    
    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 360)))
    def _invoke(self, **kwargs):
        """
        Invoke a downstream agent and collect its output.
        
        Args:
            **kwargs: Additional context from the parent workflow
            
        Returns:
            dict: Agent output including answer, references, and metadata
        """
        if self.check_if_canceled("AgentInvoke processing"):
            return
        
        # Get tenant ID from canvas
        tenant_id = self._canvas.get_tenant_id()
        if not tenant_id:
            error_msg = "Tenant ID not available in canvas context"
            logging.error(error_msg)
            self.set_output("_ERROR", error_msg)
            return error_msg
        
        # Resolve agent_id if it's a variable reference
        agent_id = self._param.agent_id
        if agent_id and agent_id.startswith("{") and agent_id.endswith("}"):
            try:
                agent_id = self._canvas.get_variable_value(agent_id)
            except Exception as e:
                logging.warning(f"Failed to resolve agent_id variable: {e}")
        
        if not agent_id:
            error_msg = "Agent ID is required but was not provided"
            logging.error(error_msg)
            self.set_output("_ERROR", error_msg)
            return error_msg
        
        # Resolve query from parameter or variable
        query = self._param.query
        if query and query.startswith("{") and query.endswith("}"):
            try:
                query = self._canvas.get_variable_value(query)
            except Exception as e:
                logging.warning(f"Failed to resolve query variable: {e}")
                query = ""
        
        # Build inputs dictionary by resolving all variable references
        inputs = {}
        if self._param.inputs:
            for key, value in self._param.inputs.items():
                if isinstance(value, str) and value.startswith("{") and value.endswith("}"):
                    try:
                        inputs[key] = self._canvas.get_variable_value(value)
                    except Exception as e:
                        logging.warning(f"Failed to resolve input '{key}' variable: {e}")
                        inputs[key] = value
                else:
                    inputs[key] = value
        
        # Determine session_id
        session_id = None
        if not self._param.create_new_session:
            session_id = self._param.session_id
            if session_id and session_id.startswith("{") and session_id.endswith("}"):
                try:
                    session_id = self._canvas.get_variable_value(session_id)
                except Exception:
                    session_id = None
        
        agent_name = self._param.agent_name or agent_id
        logging.info(f"AgentInvoke: Invoking agent '{agent_name}' (ID: {agent_id}) with query: {query[:100]}...")
        
        try:
            # Import here to avoid circular dependencies
            from api.db.services.canvas_service import completion as agent_completion
            
            # Invoke the agent and collect output
            result = self._invoke_agent_sync(
                agent_completion,
                tenant_id=tenant_id,
                agent_id=agent_id,
                query=query,
                inputs=inputs,
                session_id=session_id,
                timeout=self._param.timeout_seconds
            )
            
            if result.get("error"):
                error_msg = result["error"]
                logging.error(f"AgentInvoke: Agent '{agent_name}' failed: {error_msg}")
                self.set_output("_ERROR", error_msg)
                return error_msg
            
            # Set outputs
            self.set_output("answer", result.get("answer", ""))
            self.set_output("reference", result.get("reference", {}))
            self.set_output("session_id", result.get("session_id", ""))
            self.set_output("metadata", result.get("metadata", {}))
            self.set_output("result", result.get("answer", ""))  # Alias for compatibility
            
            logging.info(f"AgentInvoke: Agent '{agent_name}' completed successfully")
            return result.get("answer", "")
            
        except Exception as e:
            error_msg = f"Failed to invoke agent '{agent_name}': {str(e)}"
            logging.exception(error_msg)
            self.set_output("_ERROR", error_msg)
            return error_msg
    
    def _invoke_agent_sync(self, completion_func, tenant_id, agent_id, query, inputs, session_id, timeout):
        """
        Synchronously invoke an agent and wait for completion.
        
        This method handles the async completion generator and collects the full output.
        
        Args:
            completion_func: The agent completion function to call
            tenant_id (str): Tenant/user ID
            agent_id (str): Agent ID to invoke
            query (str): Query/question for the agent
            inputs (dict): Input parameters for the agent
            session_id (str): Optional session ID to continue conversation
            timeout (int): Timeout in seconds
            
        Returns:
            dict: Collected agent output
        """
        import asyncio
        
        # Create result container
        result = {
            "answer": "",
            "reference": {},
            "session_id": "",
            "metadata": {},
            "error": None
        }
        
        async def _run_agent():
            """Async wrapper to run the agent completion"""
            try:
                answer_parts = []
                final_session_id = None
                final_reference = {}
                metadata = {}
                
                # Run the agent completion generator
                async for event_data in completion_func(
                    tenant_id=tenant_id,
                    agent_id=agent_id,
                    query=query,
                    inputs=inputs,
                    session_id=session_id
                ):
                    # Check if task was canceled
                    if self.check_if_canceled("AgentInvoke processing"):
                        result["error"] = "Task was canceled"
                        return
                    
                    # Parse SSE format: "data:{json}\n\n"
                    if isinstance(event_data, str) and event_data.startswith("data:"):
                        json_str = event_data[5:].strip()
                        try:
                            event = json.loads(json_str)
                        except json.JSONDecodeError:
                            continue
                    else:
                        event = event_data
                    
                    # Extract session_id if present
                    if "session_id" in event:
                        final_session_id = event["session_id"]
                    
                    # Handle different event types
                    event_type = event.get("event", "")
                    data = event.get("data", {})
                    
                    if event_type == "message":
                        # Accumulate message content
                        content = data.get("content", "")
                        if content:
                            answer_parts.append(content)
                    
                    elif event_type == "message_end":
                        # Final message with references
                        if data and data.get("reference"):
                            final_reference = data["reference"]
                    
                    elif event_type == "workflow_finished":
                        # Agent completed successfully
                        outputs = data.get("outputs", {})
                        metadata = {
                            "elapsed_time": data.get("elapsed_time", 0),
                            "created_at": data.get("created_at", time.time())
                        }
                        # If outputs contain content, use it as the answer
                        if isinstance(outputs, dict) and outputs.get("content"):
                            answer_parts = [outputs["content"]]
                        break
                    
                    elif event_type == "error":
                        # Error occurred during agent execution
                        result["error"] = data.get("message", "Unknown error")
                        return
                
                # Combine all answer parts
                result["answer"] = "".join(answer_parts)
                result["reference"] = final_reference
                result["session_id"] = final_session_id or session_id or ""
                result["metadata"] = metadata
                
            except Exception as e:
                result["error"] = str(e)
                logging.exception(f"Error during agent invocation: {e}")
        
        # Run the async function with timeout
        try:
            # Get or create event loop
            loop = self._canvas._loop if hasattr(self._canvas, '_loop') else None
            
            if loop and loop.is_running():
                # If we're already in an async context, schedule the coroutine
                future = asyncio.run_coroutine_threadsafe(_run_agent(), loop)
                future.result(timeout=timeout)
            else:
                # Run in a new event loop
                asyncio.run(asyncio.wait_for(_run_agent(), timeout=timeout))
                
        except asyncio.TimeoutError:
            result["error"] = f"Agent invocation timed out after {timeout} seconds"
            logging.error(result["error"])
        except Exception as e:
            result["error"] = f"Failed to execute agent: {str(e)}"
            logging.exception(result["error"])
        
        return result
    
    def thoughts(self) -> str:
        """Return current component status for UI display"""
        agent_name = self._param.agent_name or "agent"
        return f"Invoking {agent_name}..."

