from ragflow import RAGFlow, DataSet

from common import API_KEY, HOST_ADDRESS
from test_sdkbase import TestSdk


class TestDataset(TestSdk):
    def test_create_dataset_with_success(self):
        """
        Test creating dataset with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        ds = rag.create_dataset("God")
        if isinstance(ds, DataSet):
            assert ds.name == "God", "Name does not match."
        else:
            assert False, f"Failed to create dataset, error: {ds}"

    def test_update_dataset_with_success(self):
        """
        Test updating dataset  with success.
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        ds = rag.create_dataset("ABC")
        if isinstance(ds, DataSet):
            assert ds.name == "ABC", "Name  does not match."
            ds.name = 'DEF'
            res = ds.save()
            assert res is True, f"Failed to update dataset,  error: {res}"

        else:
            assert False, f"Failed to create dataset, error: {ds}"