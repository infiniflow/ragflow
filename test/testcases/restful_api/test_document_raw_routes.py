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

import pytest
from test.testcases.configs import INVALID_API_TOKEN
from test.testcases.restful_api.helpers.client import RestClient


@pytest.mark.p2
def test_document_image_invalid_id_contract(rest_client_noauth):
    res = rest_client_noauth.get("/documents/images/not-a-valid-image-id")
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert payload["message"] == "Image not found.", payload


@pytest.mark.p2
def test_document_download_by_id_requires_auth(create_document):
    _dataset_id, document_id = create_document("document_raw_download_auth.txt")
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.get(f"/documents/{document_id}")
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)


@pytest.mark.p2
def test_document_download_by_id_invalid_id_contract(rest_client):
    res = rest_client.get("/documents/invalid_document_id")
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert payload["message"] == "The dataset not own the document invalid_document_id.", payload


@pytest.mark.p2
def test_document_artifact_requires_auth(rest_client_noauth):
    res = rest_client_noauth.get("/documents/artifact/not-an-artifact.txt")
    assert res.status_code == 401
    payload = res.json()
    assert payload["code"] == 401, payload


@pytest.mark.p2
def test_document_artifact_rejects_unsafe_filename(rest_client):
    res = rest_client.get("/documents/artifact/not-an-artifact.exe")
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert payload["message"] == "Invalid file type.", payload
