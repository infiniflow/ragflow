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
        dataset_name = res['data']['dataset_name']

    def test_list_dataset_success(self):
        # Call the list_datasets method
        response = ragflow.list_dataset()

        code, datasets = response

        assert code == 200

    def test_list_dataset_with_checking_size_and_name(self):
        datasets_to_create = ["dataset1", "dataset2", "dataset3"]
        created_response = [ragflow.create_dataset(name) for name in datasets_to_create]

        real_name_to_create = set()
        for response in created_response:
            assert 'data' in response, "Response is missing 'data' key"
            dataset_name = response['data']['dataset_name']
            real_name_to_create.add(dataset_name)

        status_code, listed_data = ragflow.list_dataset(0, 3)
        listed_data = listed_data['data']

        listed_names = {d['name'] for d in listed_data}
        assert listed_names == real_name_to_create
        assert status_code == 200
        assert len(listed_data) == len(datasets_to_create)

    def test_list_dataset_with_getting_empty_result(self):
        datasets_to_create = []
        created_response = [ragflow.create_dataset(name) for name in datasets_to_create]

        real_name_to_create = set()
        for response in created_response:
            assert 'data' in response, "Response is missing 'data' key"
            dataset_name = response['data']['dataset_name']
            real_name_to_create.add(dataset_name)

        status_code, listed_data = ragflow.list_dataset(0, 0)
        listed_data = listed_data['data']

        listed_names = {d['name'] for d in listed_data}
        assert listed_names == real_name_to_create
        assert status_code == 200
        assert len(listed_data) == 0

    def test_list_dataset_with_creating_100_knowledge_bases(self):
        datasets_to_create = ["dataset1"] * 100
        created_response = [ragflow.create_dataset(name) for name in datasets_to_create]

        real_name_to_create = set()
        for response in created_response:
            assert 'data' in response, "Response is missing 'data' key"
            dataset_name = response['data']['dataset_name']
            real_name_to_create.add(dataset_name)

        status_code, listed_data = ragflow.list_dataset(0, 100)
        listed_data = listed_data['data']

        listed_names = {d['name'] for d in listed_data}
        assert listed_names == real_name_to_create
        assert status_code == 200
        assert len(listed_data) == 100

    def test_list_dataset_with_showing_one_dataset(self):
        response = ragflow.list_dataset(0, 1)
        code, response = response
        datasets = response['data']
        assert len(datasets) == 1

    def test_list_dataset_failure(self):
        response = ragflow.list_dataset(-1, -1)
        _, res = response
        assert "IndexError" in res['message']




