from ragflow import RAGFlow, DataSet

from common import API_KEY, HOST_ADDRESS
from test_sdkbase import TestSdk


class TestDataset(TestSdk):
    def test_create_dataset_with_success(self):
        """
        Test creating a dataset with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        ds = rag.create_dataset("God")
        if isinstance(ds, DataSet):
            assert ds.name == "God", "Name does not match."
        else:
            assert False, f"Failed to create dataset, error: {ds}"

    def test_update_dataset_with_success(self):
        """
        Test updating a dataset with success.
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        ds = rag.create_dataset("ABC")
        if isinstance(ds, DataSet):
            assert ds.name == "ABC", "Name does not match."
            ds.name = 'DEF'
            res = ds.save()
            assert res is True, f"Failed to update dataset, error: {res}"
        else:
            assert False, f"Failed to create dataset, error: {ds}"

    def test_delete_dataset_with_success(self):
        """
        Test deleting a dataset with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        ds = rag.create_dataset("MA")
        if isinstance(ds, DataSet):
            assert ds.name == "MA", "Name does not match."
            res = ds.delete()
            assert res is True, f"Failed to delete dataset, error: {res}"
        else:
            assert False, f"Failed to create dataset, error: {ds}"

    def test_list_datasets_with_success(self):
        """
        Test listing datasets with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        list_datasets = rag.list_datasets()
        assert len(list_datasets) > 0, "Do not exist any dataset"
        for ds in list_datasets:
            assert isinstance(ds, DataSet), "Existence type is not dataset."

    def test_get_detail_dataset_with_success(self):
        """
        Test getting a dataset's detail with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        ds = rag.get_dataset(name="God")
        assert isinstance(ds, DataSet), f"Failed to get dataset, error: {ds}."
        assert ds.name == "God", "Name does not match"
