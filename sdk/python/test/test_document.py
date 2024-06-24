from api.settings import RetCode
from test_sdkbase import TestSdk
from ragflow import RAGFlow
import pytest
from common import API_KEY, HOST_ADDRESS
from api.contants import NAME_LENGTH_LIMIT


class TestFile(TestSdk):
    """
    This class contains a suite of tests for the content management functionality within the dataset.
    It ensures that the following functionalities as expected:
        1. upload a local file
        2. upload a remote file
        3. download a file
        4. delete a file
        5. enable rename
        6. list files
        7. start parsing
        8. end parsing
        9. check the status of the file
        10. list the chunks
        11. delete a chunk
        12. insert a new chunk
        13. edit the status of chunk
        14. get the specific chunk
        15. retrieval test
    """

# ----------------------------upload a local file-----------------------------------------------------
    def test_upload_two_files(self):
        """
        Test uploading two files with success.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_two_files")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ["test.txt", "test1.txt"]
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert res['code'] == RetCode.SUCCESS and res['data'] is True and res['message'] == 'success'

    def test_upload_one_file(self):
        """
        Test uploading one file with success.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_one_file")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ["test.txt"]
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert res['code'] == RetCode.SUCCESS and res['data'] is True and res['message'] == 'success'

    def test_upload_without_existing_file(self):
        """
        Test uploading a file which does not exist.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_without_existing_file")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ["empty.txt"]
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert res['code'] == RetCode.DATA_ERROR and "does not exist" in res['message']

    def test_upload_file_if_dataset_does_not_exist(self):
        """
        Test uploading files if the dataset id does not exist.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        file_paths = ["test.txt"]
        res = ragflow.upload_local_file("111", file_paths)
        assert res['code'] == RetCode.DATA_ERROR and res['message'] == "Can't find this dataset"
# ----------------------------upload a remote file-----------------------------------------------------

# ----------------------------download a file-----------------------------------------------------

# ----------------------------delete a file-----------------------------------------------------

# ----------------------------enable rename-----------------------------------------------------

# ----------------------------list files-----------------------------------------------------

# ----------------------------start parsing-----------------------------------------------------

# ----------------------------stop parsing-----------------------------------------------------

# ----------------------------show the status of the file-----------------------------------------------------

# ----------------------------list the chunks of the file-----------------------------------------------------

# ----------------------------delete the chunk-----------------------------------------------------

# ----------------------------edit the status of the chunk-----------------------------------------------------

# ----------------------------insert a new chunk-----------------------------------------------------

# ----------------------------upload a file-----------------------------------------------------

# ----------------------------get a specific chunk-----------------------------------------------------

# ----------------------------retrieval test-----------------------------------------------------
