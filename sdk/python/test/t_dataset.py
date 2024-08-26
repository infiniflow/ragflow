from ragflow import RAGFlow

from common import API_KEY, HOST_ADDRESS
from test_sdkbase import TestSdk


class TestDataset(TestSdk):
    def test_create_dataset_with_success(self):
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        ds = rag.create_dataset("God")
        assert ds is not None, "The dataset creation failed, returned None."
        assert ds.name == "God", "Dataset name does not match."

    def test_delete_one_file(self):
        """
        Test deleting one file with success.
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        ds = rag.create_dataset("ABC")
        assert ds is not None, "Failed to create dataset"
        assert ds.name == "ABC", "Dataset name mismatch"
        delete_result = ds.delete()
        assert delete_result is True, "Failed to delete dataset"
