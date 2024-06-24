from test_sdkbase import TestSdk
import ragflow
from ragflow.ragflow import RAGFLow
import pytest
from unittest.mock import MagicMock
from common import API_KEY, HOST_ADDRESS


class TestBasic(TestSdk):

    def test_version(self):
        print(ragflow.__version__)

    # def test_create_dataset(self):
    #     res = RAGFLow(API_KEY, HOST_ADDRESS).create_dataset('abc')
    #     print(res)
    #
    # def test_delete_dataset(self):
    #     assert RAGFLow('123', 'url').delete_dataset('abc') == 'abc'
    #
    # def test_list_dataset_success(self, ragflow_instance, monkeypatch):
    #     # Mocking the response of requests.get method
    #     mock_response = MagicMock()
    #     mock_response.status_code = 200
    #     mock_response.json.return_value = {'datasets': [{'id': 1, 'name': 'dataset1'}, {'id': 2, 'name': 'dataset2'}]}
    #
    #     # Patching requests.get to return the mock_response
    #     monkeypatch.setattr("requests.get", MagicMock(return_value=mock_response))
    #
    #     # Call the method under test
    #     result = ragflow_instance.list_dataset()
    #
    #     # Assertion
    #     assert result == [{'id': 1, 'name': 'dataset1'}, {'id': 2, 'name': 'dataset2'}]
    #
    # def test_list_dataset_failure(self, ragflow_instance, monkeypatch):
    #     # Mocking the response of requests.get method
    #     mock_response = MagicMock()
    #     mock_response.status_code = 404  # Simulating a failed request
    #
    #     # Patching requests.get to return the mock_response
    #     monkeypatch.setattr("requests.get", MagicMock(return_value=mock_response))
    #
    #     # Call the method under test
    #     result = ragflow_instance.list_dataset()
    #
    #     # Assertion
    #     assert result is None
