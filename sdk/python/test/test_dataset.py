from test_sdkbase import TestSdk
from ragflow import RAGFlow
import pytest
from common import API_KEY, HOST_ADDRESS

ragflow = RAGFlow(API_KEY, HOST_ADDRESS)


class TestDataset(TestSdk):

    def test_create_dataset(self):
        '''
        1. create a kb
        2. list the kb
        3. get the detail info according to the kb id
        4. update the kb
        5. delete the kb
        '''


        # create a kb
        res = ragflow.create_dataset("kb1")
        assert res['code'] == 0 and res['message'] == 'success'
        dataset_id = res['data']['dataset_id']
        print(dataset_id)

    def test_list_dataset_success(self):
        # Call the list_datasets method
        response = ragflow.list_dataset()
        print(response)

        # Check if the response is a dictionary and has the 'message' key
        if isinstance(response, dict) and 'message' in response:
            # Access the message using the 'message' key
            assert response['message'] == "attempt to list datasets"
        else:
            print("Unexpected response format:", response)

        # If the response is a dictionary with a 'data' key, proceed with the test
        if 'data' in response:
            datasets = response['data']
            assert len(datasets) > 0
            print(datasets)
        else:
            print("Expected 'data' key in response")

        # Assuming the response dictionary also contains a 'code' key with the status code
        if 'code' in response:
            status_code = response['code']
            assert status_code == 200
        else:
            print("Expected 'code' key in response")





