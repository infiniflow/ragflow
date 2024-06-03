from test_sdkbase import TestSdk
import ragflow
from ragflow.ragflow import RAGFlow
import pytest


class TestCase(TestSdk):
    def test_version(self):
        print(ragflow.__version__)

    def test_create_database(self):
        obj = RAGFlow('123', 'base_url')
        assert obj.create_dataset("abc") == "abc"

