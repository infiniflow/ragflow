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
        1. upload local files
        2. upload remote files
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

# ----------------------------upload local files-----------------------------------------------------
    def test_upload_two_files(self):
        """
        Test uploading two files with success.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_two_files")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ["test_data/test.txt", "test_data/test1.txt"]
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert res['code'] == RetCode.SUCCESS and res['data'] is True and res['message'] == 'success'

    def test_upload_one_file(self):
        """
        Test uploading one file with success.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_one_file")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ["test_data/test.txt"]
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert res['code'] == RetCode.SUCCESS and res['data'] is True and res['message'] == 'success'

    def test_upload_nonexistent_files(self):
        """
        Test uploading a file which does not exist.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_nonexistent_files")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ["test_data/imagination.txt"]
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert res['code'] == RetCode.DATA_ERROR and "does not exist" in res['message']

    def test_upload_file_if_dataset_does_not_exist(self):
        """
        Test uploading files if the dataset id does not exist.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        file_paths = ["test_data/test.txt"]
        res = ragflow.upload_local_file("111", file_paths)
        assert res['code'] == RetCode.DATA_ERROR and res['message'] == "Can't find this dataset"

    def test_upload_file_without_name(self):
        """
        Test uploading files that do not have name.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_file_without_name")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ["test_data/.txt"]
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert res['code'] == RetCode.SUCCESS

    def test_upload_file_without_name1(self):
        """
        Test uploading files that do not have name.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_file_without_name")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ["test_data/.txt", "test_data/empty.txt"]
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert res['code'] == RetCode.SUCCESS

    def test_upload_files_exceeding_the_number_limit(self):
        """
        Test uploading files whose number exceeds the limit.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_files_exceeding_the_number_limit")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ["test_data/test.txt", "test_data/test1.txt"] * 256
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert (res['message'] ==
                'You try to upload 512 files, which exceeds the maximum number of uploading files: 256'
                and res['code'] == RetCode.DATA_ERROR)

    def test_upload_files_without_files(self):
        """
        Test uploading files without files.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_files_without_files")
        dataset_id = created_res['data']['dataset_id']
        file_paths = [None]
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert (res['message'] == 'None is not string.' and res['code'] == RetCode.ARGUMENT_ERROR)

    def test_upload_files_with_two_files_with_same_name(self):
        """
        Test uploading files with the same name.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_files_with_two_files_with_same_name")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ['test_data/test.txt'] * 2
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert (res['message'] == 'success' and res['code'] == RetCode.SUCCESS)

    def test_upload_files_with_file_paths(self):
        """
        Test uploading files with only specifying the file path's repo.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_files_with_file_paths")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ['test_data/']
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert (res['message'] == 'The file test_data/ does not exist' and res['code'] == RetCode.DATA_ERROR)

    def test_upload_files_with_remote_file_path(self):
        """
        Test uploading files with remote files.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_upload_files_with_remote_file_path")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ['https://github.com/genostack/ragflow']
        res = ragflow.upload_local_file(dataset_id, file_paths)
        assert res['code'] == RetCode.ARGUMENT_ERROR and res['message'] == 'Remote files have not unsupported.'

# ----------------------------delete a file-----------------------------------------------------
    def test_delete_one_file(self):
        """
        Test deleting one file with success.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_delete_one_file")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ["test_data/test.txt"]
        res = ragflow.upload_local_file(dataset_id, file_paths)
        # get the doc_id
        data = res['data'][0]
        doc_id = data['id']
        # delete the files
        deleted_res = ragflow.delete_files(doc_id, dataset_id)
        # assert value
        assert deleted_res['code'] == RetCode.SUCCESS and deleted_res['data'] is True

    def test_delete_document_with_not_existing_document(self):
        """
        Test deleting a document that does not exist with failure.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_delete_document_with_not_existing_document")
        dataset_id = created_res['data']['dataset_id']
        res = ragflow.delete_files("111", dataset_id)
        assert res['code'] == RetCode.DATA_ERROR and res['message'] == 'Document 111 not found!'

    def test_delete_document_with_creating_100_documents_and_deleting_100_documents(self):
        """
        Test deleting documents when uploading 100 docs and deleting 100 docs.
        """
        # upload 100 docs
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_delete_one_file")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ["test_data/test.txt"] * 100
        res = ragflow.upload_local_file(dataset_id, file_paths)

        # get the doc_id
        data = res['data']
        for d in data:
            doc_id = d['id']
            # delete the files
            deleted_res = ragflow.delete_files(doc_id, dataset_id)
            # assert value
            assert deleted_res['code'] == RetCode.SUCCESS and deleted_res['data'] is True

    def test_delete_document_from_nonexistent_dataset(self):
        """
        Test deleting documents from a non-existent dataset
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        created_res = ragflow.create_dataset("test_delete_one_file")
        dataset_id = created_res['data']['dataset_id']
        file_paths = ["test_data/test.txt"]
        res = ragflow.upload_local_file(dataset_id, file_paths)
        # get the doc_id
        data = res['data'][0]
        doc_id = data['id']
        # delete the files
        deleted_res = ragflow.delete_files(doc_id, "000")
        # assert value
        assert (deleted_res['code'] == RetCode.ARGUMENT_ERROR and deleted_res['message'] ==
                f'The document {doc_id} is not in the dataset: 000, but in the dataset: {dataset_id}.')

    def test_delete_document_which_is_located_in_other_dataset(self):
        """
        Test deleting a document which is located in other dataset.
        """
        ragflow = RAGFlow(API_KEY, HOST_ADDRESS)
        # upload a document
        created_res = ragflow.create_dataset("test_delete_document_which_is_located_in_other_dataset")
        created_res_id = created_res['data']['dataset_id']
        file_paths = ["test_data/test.txt"]
        res = ragflow.upload_local_file(created_res_id, file_paths)
        # other dataset
        other_res = ragflow.create_dataset("other_dataset")
        other_dataset_id = other_res['data']['dataset_id']
        # get the doc_id
        data = res['data'][0]
        doc_id = data['id']
        # delete the files from the other dataset
        deleted_res = ragflow.delete_files(doc_id, other_dataset_id)
        # assert value
        assert (deleted_res['code'] == RetCode.ARGUMENT_ERROR and deleted_res['message'] ==
                f'The document {doc_id} is not in the dataset: {other_dataset_id}, but in the dataset: {created_res_id}.')

# ----------------------------download a file-----------------------------------------------------

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
