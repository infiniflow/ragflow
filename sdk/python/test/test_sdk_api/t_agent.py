from ragflow_sdk import RAGFlow,Agent
from common import HOST_ADDRESS
import pytest

@pytest.mark.skip(reason="")
def test_list_agents_with_success(get_api_key_fixture):
    API_KEY=get_api_key_fixture
    rag = RAGFlow(API_KEY,HOST_ADDRESS)
    rag.list_agents()


@pytest.mark.skip(reason="")
def test_converse_with_agent_with_success(get_api_key_fixture):
    API_KEY = "ragflow-BkOGNhYjIyN2JiODExZWY5MzVhMDI0Mm"
    agent_id = "ebfada2eb2bc11ef968a0242ac120006"
    rag = RAGFlow(API_KEY,HOST_ADDRESS)
    lang = "Chinese"
    file = "How is the weather tomorrow?"
    Agent.ask(agent_id=agent_id,rag=rag,lang=lang,file=file)
