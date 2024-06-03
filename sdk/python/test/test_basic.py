from test_sdkbase import TestSdk
import ragflow
from ragflow.ragflow import RAGFLow
import pytest


class TestCase(TestSdk):
    def test_version(self):
        print(ragflow.__version__)

    def test_create_dataset(self):
        assert ragflow.ragflow.RAGFLow('123', 'url').create_dataset('abc') == 'abc'

    def test_delete_dataset(self):
        assert ragflow.ragflow.RAGFLow('123', 'url').delete_dataset('abc') == 'abc'