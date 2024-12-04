from ragflow_sdk import RAGFlow,Agent
from common import HOST_ADDRESS
import pytest

@pytest.mark.skip(reason="")
def test_list_agents_with_success(get_api_key_fixture):
    API_KEY=get_api_key_fixture
    rag = RAGFlow(API_KEY,HOST_ADDRESS)
    rag.list_agents()