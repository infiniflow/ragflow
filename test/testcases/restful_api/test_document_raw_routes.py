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


@pytest.mark.p2
def test_document_image_invalid_id_contract(rest_client_noauth):
    res = rest_client_noauth.get("/documents/images/not-a-valid-image-id")
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert payload["message"] == "Image not found.", payload


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
