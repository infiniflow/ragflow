import pytest
from unittest.mock import patch, MagicMock, AsyncMock

@pytest.mark.p1
@pytest.mark.asyncio
async def test_agent_hallucination_on_failed_retrieval():
    prompt = "Use the retrieval pipeline to answer this question. If it fails, reply ACTION_NOT_PERFORMED."
    
    # Isolate the Nuclear Option so it doesn't globally poison other tests
    mock_modules = {
        'infinity': MagicMock(),
        'infinity.rag_tokenizer': MagicMock(),
        'infinity.common': MagicMock(),
        'infinity.errors': MagicMock(),
        'infinity.index': MagicMock(),
        'google': MagicMock(),
        'google.cloud': MagicMock(),
        'google.cloud.storage': MagicMock(),
        'google.protobuf': MagicMock(),
        'google.protobuf.internal': MagicMock(),
        'langfuse': MagicMock(),
        'google.api_core': MagicMock(),
        'google.api_core.exceptions': MagicMock()
    }
    
    with patch.dict('sys.modules', mock_modules):
        # Import the Agent INSIDE the isolated environment
        from agent.component.agent_with_tools import Agent
        
        # 1. THE NUCLEAR OPTION
        agent = Agent.__new__(Agent)
        agent.id = "test"
        agent._id = "test"
        agent._param = MagicMock()
        agent._canvas = MagicMock()
        
        agent._canvas.get_component = MagicMock(return_value=None) 
        agent.check_if_canceled = MagicMock(return_value=False)
        agent._prepare_prompt_variables = MagicMock(return_value=("", [], ""))
        agent._get_output_schema = MagicMock(return_value=None)
        agent.exception_handler = MagicMock(return_value=None)
        agent._fit_messages = MagicMock(return_value=[{"role": "user", "content": prompt}])
        agent._append_system_prompt = MagicMock()
        agent.set_output = MagicMock()
        agent._collect_tool_artifact_markdown = MagicMock(return_value="")

        # 2. THE SABOTAGE 
        # Simulating an empty tool (never ran)
        mock_tool_empty = MagicMock()
        mock_tool_empty._param.outputs = {} 
        
        # Simulating a failed tool (ran, but errored out)
        mock_tool_failed = MagicMock()
        mock_tool_failed._param.outputs = {"_ERROR": "Connection timeout", "_elapsed_time": 1.5}
        
        agent.tools = {"empty_tool": mock_tool_empty, "failed_tool": mock_tool_failed}
        
        # 3. THE MOCK AI & EXECUTION
        with patch('agent.component.agent_with_tools.Agent._generate_async', new_callable=AsyncMock) as mock_generate:
            mock_generate.return_value = "I searched the available context and found the result."
            response = await agent._invoke_async(user_prompt=prompt)
            
        # 4. THE ASSERTION
        assert response is not None, "Test Failed: The function exited early and returned None."
        assert response == "ACTION_NOT_PERFORMED", f"Agent hallucinated! Response: {response}"