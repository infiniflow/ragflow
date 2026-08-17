#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import pytest
import requests
from configs import HOST_ADDRESS, VERSION, INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth
from common import HEADERS

DATASETS_API_URL = f"/api/{VERSION}/datasets"


def get_dataset_metadata_config(auth, dataset_id, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/metadata/config"
    res = requests.get(url=url, headers=headers, auth=auth)
    return res.json()


def update_dataset_metadata_config(auth, dataset_id, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/metadata/config"
    res = requests.put(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


def update_document_metadata_config(auth, dataset_id, document_id, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/documents/{document_id}/metadata/config"
    res = requests.put(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


@pytest.mark.p1
class TestDatasetMetadataConfigAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                401,
                "<Unauthorized '401: Unauthorized'>",
            ),
        ],
    )
    def test_get_metadata_config_auth_invalid(self, invalid_auth, expected_code, expected_message):
        res = get_dataset_metadata_config(invalid_auth, "dataset_id")
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res

    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                401,
                "<Unauthorized '401: Unauthorized'>",
            ),
        ],
    )
    def test_update_metadata_config_auth_invalid(self, invalid_auth, expected_code, expected_message):
        res = update_dataset_metadata_config(invalid_auth, "dataset_id", {})
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


@pytest.mark.usefixtures("clear_datasets")
class TestDatasetMetadataConfig:
    @pytest.mark.p2
    def test_get_metadata_config_success(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = get_dataset_metadata_config(HttpApiAuth, dataset_id)
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_get_metadata_config_invalid_dataset(self, HttpApiAuth):
        res = get_dataset_metadata_config(HttpApiAuth, "invalid_dataset_id")
        assert res["code"] != 0, res

    @pytest.mark.p2
    def test_update_metadata_config_missing_payload(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = update_dataset_metadata_config(HttpApiAuth, dataset_id)
        assert res["code"] != 0, res

    @pytest.mark.p2
    def test_update_metadata_config_invalid_dataset(self, HttpApiAuth):
        res = update_dataset_metadata_config(HttpApiAuth, "invalid_dataset_id", {"fields": []})
        assert res["code"] != 0, res


@pytest.mark.p1
class TestDocumentMetadataConfigAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                401,
                "<Unauthorized '401: Unauthorized'>",
            ),
        ],
    )
    def test_update_document_metadata_config_auth_invalid(self, invalid_auth, expected_code, expected_message):
        res = update_document_metadata_config(invalid_auth, "dataset_id", "document_id", {})
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


@pytest.mark.usefixtures("clear_datasets")
class TestDocumentMetadataConfig:
    @pytest.mark.p2
    def test_update_document_metadata_config_not_found(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = update_document_metadata_config(HttpApiAuth, dataset_id, "nonexistent_doc_id", {})
        assert res["code"] != 0, res

    @pytest.mark.p2
    def test_update_document_metadata_config_invalid_dataset(self, HttpApiAuth, add_document_func):
        _, doc_id = add_document_func
        res = update_document_metadata_config(HttpApiAuth, "invalid_dataset_id", doc_id, {})
        assert res["code"] != 0, res

    @pytest.mark.p2
    def test_update_document_metadata_config_invalid_document(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = update_document_metadata_config(HttpApiAuth, dataset_id, "invalid_doc_id", {})
        assert res["code"] != 0, res
