#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""Unit tests for Bedrock credential parsing.

Bedrock credentials are stored as a JSON object. A bare string in that field
used to reach ``json.loads`` unguarded, so an operator who pasted a plain AWS
access key got a ``JSONDecodeError`` traceback out of the connector internals
instead of being told what the field expects.
"""

import json
from unittest.mock import MagicMock, patch

import pytest

from rag.llm.key_utils import _resolve_bedrock_credentials
from rag.llm.rerank_model import BedrockRerank

pytestmark = pytest.mark.p1

CREDENTIALS = {
    "auth_mode": "access_key_secret",
    "bedrock_region": "us-east-1",
    "bedrock_ak": "AKIA_TEST",
    "bedrock_sk": "secret_test",
}


def test_json_object_is_parsed():
    assert _resolve_bedrock_credentials(json.dumps(CREDENTIALS)) == CREDENTIALS


def test_dict_is_passed_through():
    assert _resolve_bedrock_credentials(CREDENTIALS) == CREDENTIALS


@pytest.mark.parametrize("key", ["AKIAIOSFODNN7EXAMPLE", "", None, 42])
def test_key_that_is_not_json_reports_the_expected_schema(key):
    with pytest.raises(ValueError, match="auth_mode"):
        _resolve_bedrock_credentials(key)


@pytest.mark.parametrize("key", ["[]", '"AKIAIOSFODNN7EXAMPLE"', "42"])
def test_json_that_is_not_an_object_is_rejected(key):
    with pytest.raises(ValueError, match="auth_mode"):
        _resolve_bedrock_credentials(key)


def test_connector_reports_the_schema_instead_of_a_decode_error():
    with patch("boto3.client") as client_factory:
        client_factory.return_value = MagicMock()
        with pytest.raises(ValueError, match="auth_mode"):
            BedrockRerank("AKIAIOSFODNN7EXAMPLE", "amazon.rerank-v1:0")


def test_valid_key_still_reaches_the_connector():
    with patch("boto3.client") as client_factory:
        client_factory.return_value = MagicMock()
        BedrockRerank(json.dumps(CREDENTIALS), "amazon.rerank-v1:0")
    assert client_factory.call_args.kwargs["aws_access_key_id"] == "AKIA_TEST"
