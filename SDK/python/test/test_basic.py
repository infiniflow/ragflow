from test_sdkbase import TestSdk
import ragflow
import pytest


class TestCase(TestSdk):
    def test_version(self):
        print(ragflow.__version__)