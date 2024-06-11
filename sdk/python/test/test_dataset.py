from test_sdkbase import TestSdk
import ragflow
from ragflow.ragflow import RAGFLow
import pytest
from unittest.mock import MagicMock
from common import API_KEY, HOST_ADDRESS

class TestDataset(TestSdk):

    def test_create_dataset(self):
        '''
        1. create a kb
        2. list the kb
        3. get the detail info according to the kb id
        4. update the kb
        5. delete the kb
        '''
        ragflow = RAGFLow(API_KEY, HOST_ADDRESS)

        # create a kb
        res = ragflow.create_dataset("kb1")
        assert res['code'] == 0 and res['message'] == 'success'
        dataset_id = res['data']['dataset_id']
        print(dataset_id)

        # TODO: list the kb