import sys
import pytest
from unittest.mock import patch, MagicMock, AsyncMock

# --- 1. THE ANTI-DEPENDENCY HELL SHIELD (GLOBAL) ---
# We run this FIRST to prevent circular import crashes and missing packages
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

# --- 2. RAGFLOW IMPORTS ---
# Import the Agent and tracker ONCE after the shield is completely up
from agent.component.agent_with_tools import Agent, _tool_call_tracker

def setup_agent(prompt):
    """Helper to initialize the mocked Agent for our tests"""
    agent = Agent.__new__(Agent)
    agent.id = "test"
    agent._id = "test"
    agent._param = MagicMock()
    agent._canvas = MagicMock()
    agent.toolcall_session = MagicMock()
    agent.toolcall_session.callback = MagicMock()
    
    agent._canvas.get_component = MagicMock(return_value=None) 
    agent.check_if_canceled = MagicMock(return_value=False)
    agent._prepare_prompt_variables = MagicMock(return_value=("", [], ""))
    agent._get_output_schema = MagicMock(return_value=None)
    agent.exception_handler = MagicMock(return_value=None)
    agent._fit_messages = MagicMock(return_value=[{"role": "user", "content": prompt}])
    agent._append_system_prompt = MagicMock()
    agent.set_output = MagicMock()
    agent._collect_tool_artifact_markdown = MagicMock(return_value="")
    return agent

@pytest.mark.p1
@pytest.mark.asyncio
async def test_agent_hallucination_on_failed_retrieval():
    """Scenario 1: Tool is called, but fails. AI lies. Trapdoor MUST catch it."""
    prompt = "Find my password."
    agent = setup_agent(prompt)
    
    mock_tool_empty = MagicMock()
    mock_tool_empty._param.outputs = {} 
    agent.tools = {"empty_tool": mock_tool_empty}
    
    with patch('agent.component.agent_with_tools.Agent._generate_async', new_callable=AsyncMock) as mock_generate:
        async def side_effect(*args, **kwargs):
            # Manually flip the tracker switch since we mocked the real function
            _tool_call_tracker.set(True)
            return "I found your password, it is 1234."
        mock_generate.side_effect = side_effect
        
        response = await agent._invoke_async(user_prompt=prompt)
        
    assert response == "ACTION_NOT_PERFORMED", "Failed: Trapdoor did not catch hallucination."

@pytest.mark.p1
@pytest.mark.asyncio
async def test_agent_allows_small_talk():
    """Scenario 2: User says Hello. No tool is called. Trapdoor MUST NOT fire."""
    prompt = "Hello there!"
    agent = setup_agent(prompt)
    
    mock_tool = MagicMock()
    mock_tool._param.outputs = {} 
    agent.tools = {"some_tool": mock_tool}
    
    with patch('agent.component.agent_with_tools.Agent._generate_async', new_callable=AsyncMock) as mock_generate:
        expected_response = "Hi! How can I help you today?"
        mock_generate.return_value = expected_response
        
        # We do NOT set the tracker here because no tool was called
        response = await agent._invoke_async(user_prompt=prompt)
        
    assert response == expected_response, "Failed: Trapdoor destroyed a valid small-talk response."

@pytest.mark.p1
@pytest.mark.asyncio
async def test_agent_allows_successful_retrieval():
    """Scenario 3: Tool is called and succeeds. Trapdoor MUST NOT fire."""
    prompt = "What is the weather?"
    agent = setup_agent(prompt)
    
    mock_tool = MagicMock()
    mock_tool._param.outputs = {"temperature": "72F"} 
    agent.tools = {"weather_tool": mock_tool}
    
    with patch('agent.component.agent_with_tools.Agent._generate_async', new_callable=AsyncMock) as mock_generate:
        expected_response = "The weather is 72 degrees."
        async def side_effect(*args, **kwargs):
            # Manually flip the tracker switch 
            _tool_call_tracker.set(True)
            return expected_response
        mock_generate.side_effect = side_effect
        
        response = await agent._invoke_async(user_prompt=prompt)
        
    assert response == expected_response, "Failed: Trapdoor destroyed a valid tool-backed response."