import pytest
from unittest.mock import patch, MagicMock, AsyncMock
import sys

# Silence the external libraries
sys.modules['infinity'] = MagicMock()
sys.modules['infinity.rag_tokenizer'] = MagicMock()
sys.modules['infinity.common'] = MagicMock()
sys.modules['infinity.errors'] = MagicMock()
sys.modules['infinity.index'] = MagicMock()
sys.modules['google'] = MagicMock()
sys.modules['google.cloud'] = MagicMock()
sys.modules['google.cloud.storage'] = MagicMock()
sys.modules['google.protobuf'] = MagicMock()
sys.modules['google.protobuf.internal'] = MagicMock()
sys.modules['langfuse'] = MagicMock()
sys.modules['google.api_core'] = MagicMock()
sys.modules['google.api_core.exceptions'] = MagicMock()

from agent.component.agent_with_tools import Agent

@pytest.mark.asyncio
async def test_agent_hallucination_on_failed_retrieval():
    prompt = "Use the retrieval pipeline to answer this question. If it fails, reply ACTION_NOT_PERFORMED."
    
    
    agent = Agent.__new__(Agent)
    agent._id = "test"
    agent._param = MagicMock()
    agent._canvas = MagicMock()
    
    # Close all the early-exit trap doors
    agent._canvas.get_component = MagicMock(return_value=None) 
    agent.check_if_canceled = MagicMock(return_value=False)
    agent._prepare_prompt_variables = MagicMock(return_value=("", [], ""))
    agent._get_output_schema = MagicMock(return_value=None)
    agent.exception_handler = MagicMock(return_value=None)
    agent._fit_messages = MagicMock(return_value=[{"role": "user", "content": prompt}])
    agent._append_system_prompt = MagicMock()
    agent.set_output = MagicMock()
    agent._collect_tool_artifact_markdown = MagicMock(return_value="")

    # ---------------------------------------------------------
    # 2. THE SABOTAGE (Simulate Empty Database)
    # ---------------------------------------------------------
    mock_tool = MagicMock()
    mock_tool._param.outputs = {}  # This triggers your Layer 2 trapdoor
    agent.tools = {"fake_search_tool": mock_tool}
    
    # ---------------------------------------------------------
    # 3. THE MOCK AI & EXECUTION
    # ---------------------------------------------------------
    with patch('agent.component.agent_with_tools.Agent._generate_async', new_callable=AsyncMock) as mock_generate:
        # Force the AI to hallucinate
        mock_generate.return_value = "I searched the available context and found the result."
        
        # Trigger the agent pipeline
        response = await agent._invoke_async(user_prompt=prompt)
        
    # ---------------------------------------------------------
    # 4. THE ASSERTION
    # ---------------------------------------------------------
    assert response is not None, "Test Failed: The function exited early and returned None."
    assert "ACTION_NOT_PERFORMED" in response, f"Agent hallucinated! Response: {response}"