"""Unit tests for Agent tool execution trapdoors."""
import logging
from unittest.mock import MagicMock, patch
import pytest

logger = logging.getLogger(__name__)

@pytest.fixture
def agent_setup():
    """Sets up a barebones Agent."""
    logger.debug("Setting up agent_setup fixture: Initializing barebones Agent.")
    from agent.component.agent_with_tools import Agent, ToolExecutionState
    from agent.canvas import Canvas

    canvas_mock = MagicMock(spec=Canvas)
    canvas_mock.get_tenant_id.return_value = "test_tenant"
    canvas_mock.is_canceled.return_value = False
    
    param_mock = MagicMock()
    param_mock.llm_id = "test_llm"
    param_mock.max_retries = 0
    param_mock.outputs = {} 
    param_mock.tools = []
    param_mock.mcp = []
    param_mock.prompt = ""
    
    with patch('agent.component.llm.get_model_config_by_type_and_name', return_value={}), \
         patch('agent.component.agent_with_tools.get_model_config_by_type_and_name', return_value={}), \
         patch('agent.component.llm.LLMBundle'), \
         patch('agent.component.agent_with_tools.LLMBundle'), \
         patch('api.db.services.tenant_llm_service.TenantLLMService.llm_id2llm_type', return_value='chat'):
        
        agent = Agent(canvas=canvas_mock, id="test_agent", param=param_mock)
        agent.tools = {"dummy_tool": MagicMock()}
        
        async def mock_generate_async(*args, **kwargs):
            return "Mocked LLM Response"
        agent._generate_async = mock_generate_async
        
        async def mock_stream(*args, **kwargs):
            for _ in []:
                yield ""
            
        agent._generate_streamly = mock_stream
        agent._prepare_prompt_variables = MagicMock(return_value=("System Prompt", [{"role": "user", "content": "Test"}], {}))

        canvas_mock.get_component.return_value = {"downstream": []}
        
        logger.debug("Agent setup complete. Yielding Agent and ToolExecutionState.")
        yield agent, ToolExecutionState


@pytest.mark.asyncio
@pytest.mark.p1
async def test_invoke_async_empty_result_trapdoor(agent_setup):
    """CASE 1: Proves the trapdoor overrides the LLM output and returns safe structured data on EMPTY_RESULT."""
    logger.info("Executing test_invoke_async_empty_result_trapdoor")
    mock_agent, ToolExecutionState = agent_setup
    mock_agent._param.outputs["structured"] = {"properties": {"summary": {"type": "string"}}}
 
    with patch.object(mock_agent, '_get_tool_execution_state', return_value=ToolExecutionState.EMPTY_RESULT) as mock_get_state:
        result = await mock_agent._invoke_async(user_prompt="Test query")
        
        logger.debug("Evaluating trapdoor assertions for EMPTY_RESULT state.")
        mock_get_state.assert_called_once()
        assert result == {}
        assert mock_agent.output("structured") == {}
        assert mock_agent.output("content") == "ACTION_NOT_PERFORMED"
        assert mock_agent.error() is None
    logger.info("test_invoke_async_empty_result_trapdoor passed successfully.")

@pytest.mark.asyncio
@pytest.mark.p1
async def test_stream_output_empty_result_trapdoor(agent_setup):
    """CASE 2: Proves the streaming generator intercepts EMPTY_RESULT and yields the sentinel string."""
    logger.info("Executing test_stream_output_empty_result_trapdoor")
    mock_agent, ToolExecutionState = agent_setup
    with patch.object(mock_agent, '_get_tool_execution_state', return_value=ToolExecutionState.EMPTY_RESULT) as mock_get_state:
        
        stream_generator = mock_agent.stream_output_with_tools_async(
            prompt="system prompt", 
            msg=[{"role": "user", "content": "Test"}]
        )
        
        chunks = [chunk async for chunk in stream_generator]
        
        logger.debug("Evaluating trapdoor assertions for streaming EMPTY_RESULT state.")
        mock_get_state.assert_called_once()
        assert chunks == ["ACTION_NOT_PERFORMED"]
        assert mock_agent.output("content") == "ACTION_NOT_PERFORMED"
    logger.info("test_stream_output_empty_result_trapdoor passed successfully.")

@pytest.mark.asyncio
@pytest.mark.p1
async def test_invoke_async_real_error_bypasses_trapdoor(agent_setup):
    """CASE 3: Proves that real tool crashes bypass the trapdoor and trigger the native error channel."""
    logger.info("Executing test_invoke_async_real_error_bypasses_trapdoor")
    mock_agent, ToolExecutionState = agent_setup
    
    async def mock_error_generate(*args, **kwargs):
        return "**ERROR** Database Timeout"
    mock_agent._generate_async = mock_error_generate
    
    with patch.object(mock_agent, '_get_tool_execution_state', return_value=ToolExecutionState.ERROR) as mock_get_state:
        await mock_agent._invoke_async(user_prompt="Test query")
        
        logger.debug("Evaluating assertions for ERROR bypass state.")
        mock_get_state.assert_called_once()
        assert mock_agent.output("content") != "ACTION_NOT_PERFORMED"
        assert mock_agent.error() == "**ERROR** Database Timeout"
    logger.info("test_invoke_async_real_error_bypasses_trapdoor passed successfully.")

@pytest.mark.asyncio
@pytest.mark.p1
async def test_invoke_async_baseline_success(agent_setup):
    """CASE 4: Proves valid tool executions flow normally without triggering trapdoors."""
    logger.info("Executing test_invoke_async_baseline_success")
    mock_agent, ToolExecutionState = agent_setup
    with patch.object(mock_agent, '_get_tool_execution_state', return_value=ToolExecutionState.SUCCESS) as mock_get_state:
        await mock_agent._invoke_async(user_prompt="Test query")
        
        logger.debug("Evaluating assertions for baseline SUCCESS state.")
        mock_get_state.assert_called_once()
        assert mock_agent.output("content") == "Mocked LLM Response"
        assert mock_agent.error() is None
    logger.info("test_invoke_async_baseline_success passed successfully.")