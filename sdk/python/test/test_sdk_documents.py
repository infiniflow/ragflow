from ragflow import RAGFlow

from api.settings import RetCode
from sdk.python.test.common import API_KEY, HOST_ADDRESS
from sdk.python.test.test_sdkbase import TestSdk


class TestDocuments(TestSdk):

    def test_upload_two_files(self):
        """
        Test uploading two files with success.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset_new("test_upload_two_files")
        dataset_id = created_res["data"]["kb_id"]
        file_paths = ["test_data/test.txt", "test_data/test1.txt"]
        res = ragflow.upload_documents_2_dataset(dataset_id, file_paths)
        assert res["retmsg"] == "success"