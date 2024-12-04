from ragflow_sdk import RAGFlow
import random
import pytest
from common import HOST_ADDRESS

def test_create_dataset_with_name(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    rag.create_dataset("test_create_dataset_with_name")

def test_create_dataset_with_duplicated_name(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    rag.create_dataset("test_create_dataset_with_duplicated_name")
    with pytest.raises(Exception) as exc_info:
        rag.create_dataset("test_create_dataset_with_duplicated_name")
    assert str(exc_info.value) == "Duplicated dataset name in creating dataset."

def test_create_dataset_with_random_chunk_method(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    valid_chunk_methods = ["naive","manual","qa","table","paper","book","laws","presentation","picture","one","knowledge_graph","email"]
    random_chunk_method = random.choice(valid_chunk_methods)
    rag.create_dataset("test_create_dataset_with_random_chunk_method",chunk_method=random_chunk_method)

def test_create_dataset_with_invalid_parameter(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    valid_chunk_methods = ["naive", "manual", "qa", "table", "paper", "book", "laws", "presentation", "picture", "one",
                           "knowledge_graph", "email"]
    chunk_method = "invalid_chunk_method"
    with pytest.raises(Exception) as exc_info:
        rag.create_dataset("test_create_dataset_with_invalid_chunk_method",chunk_method=chunk_method)
    assert str(exc_info.value) == f"'{chunk_method}' is not in {valid_chunk_methods}"


def test_update_dataset_with_name(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    ds = rag.create_dataset("test_update_dataset")
    ds.update({"name": "updated_dataset"})


def test_delete_datasets_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    ds = rag.create_dataset("test_delete_dataset")
    rag.delete_datasets(ids=[ds.id])


def test_list_datasets_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    rag.create_dataset("test_list_datasets")
    rag.list_datasets()
