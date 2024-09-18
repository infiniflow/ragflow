from ragflow import RAGFlow

from sdk.python.test.common import API_KEY, HOST_ADDRESS
from sdk.python.test.test_sdkbase import TestSdk


class TestDatasets(TestSdk):

    def test_get_all_dataset_success(self):
        """
        Test listing datasets with a successful outcome.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        res = ragflow.get_all_datasets()
        assert res["retmsg"] == "success"
