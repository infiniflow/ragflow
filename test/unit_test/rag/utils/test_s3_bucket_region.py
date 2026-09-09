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

import importlib
from io import BytesIO
from unittest.mock import Mock

import pytest
from botocore.response import StreamingBody
from botocore.stub import ANY, Stubber

import common.settings  # noqa: F401 -- initialize the settings/connector import cycle.
from rag.utils import s3_conn

pytestmark = pytest.mark.p2


def make_storage(monkeypatch, tmp_path, config):
    """Create a fresh S3 singleton with isolated AWS configuration per test."""
    module = importlib.reload(s3_conn)
    # Explicit test credentials and empty config files keep the SDK independent
    # of the developer's AWS profiles. Stubber rejects any unexpected request.
    monkeypatch.setenv("AWS_CONFIG_FILE", str(tmp_path / "aws-config"))
    monkeypatch.setenv("AWS_SHARED_CREDENTIALS_FILE", str(tmp_path / "aws-credentials"))
    monkeypatch.setenv("AWS_DEFAULT_REGION", "eu-central-1")
    monkeypatch.setattr(
        module.settings,
        "S3",
        {"access_key": "test-access", "secret_key": "test-secret", "endpoint_url": "https://s3.test", **config},
    )
    storage = module.RAGFlowS3()
    # A failed base request must not reconnect or wait before we inspect the
    # stub queue. The successful write path still uses the real SDK uploader.
    monkeypatch.setattr(storage, "__open__", Mock())
    monkeypatch.setattr(module.time, "sleep", Mock())
    return storage


@pytest.mark.parametrize(
    "config,region,location",
    [
        ({"region": "eu-west-1"}, "eu-west-1", "eu-west-1"),
        ({"region_name": "ap-southeast-2"}, "ap-southeast-2", "ap-southeast-2"),
        ({"region": "us-east-1", "region_name": "eu-west-1"}, "eu-west-1", "eu-west-1"),
        ({}, "eu-central-1", "eu-central-1"),
        ({"region_name": ""}, "eu-central-1", "eu-central-1"),
        ({"region": "us-east-1"}, "us-east-1", None),
        ({"region": "auto"}, "auto", None),
    ],
)
@pytest.mark.parametrize("shared_bucket", [False, True])
def test_put_creates_bucket_in_resolved_region_then_uploads(monkeypatch, tmp_path, config, region, location, shared_bucket):
    """Verify bucket location, object routing and payload with the real SDK."""
    if shared_bucket:
        config = {**config, "bucket": "physical-bucket", "prefix_path": "documents"}
    storage = make_storage(monkeypatch, tmp_path, config)
    client = storage.conn[0]
    assert client.meta.region_name == region
    bucket = "physical-bucket" if shared_bucket else "knowledge-base"
    key = "documents/knowledge-base/file.txt" if shared_bucket else "file.txt"
    payload = b"document contents"
    uploaded = []

    def capture_upload(params, **_kwargs):
        """Record the upload bytes and rewind the body for the SDK request."""
        uploaded.append(params["Body"].read())
        params["Body"].seek(0)

    client.meta.events.register("before-parameter-build.s3.PutObject", capture_upload)
    create_params = {"Bucket": bucket}
    if location:
        create_params["CreateBucketConfiguration"] = {"LocationConstraint": location}
    try:
        with Stubber(client) as stub:
            stub.add_client_error("head_bucket", service_error_code="404", http_status_code=404, expected_params={"Bucket": bucket})
            stub.add_response("create_bucket", {}, create_params)
            stub.add_response("put_object", {}, {"Bucket": bucket, "Key": key, "Body": ANY, "ChecksumAlgorithm": ANY})
            stub.add_response("get_object", {"Body": StreamingBody(BytesIO(payload), len(payload))}, {"Bucket": bucket, "Key": key})

            storage.put("knowledge-base", "file.txt", payload)

            assert uploaded == [payload]
            assert storage.get("knowledge-base", "file.txt") == payload
            stub.assert_no_pending_responses()
            storage.__open__.assert_not_called()
    finally:
        client.close()


def test_put_to_existing_regional_bucket_skips_creation(monkeypatch, tmp_path):
    """Verify an existing regional bucket accepts uploads without creation."""
    storage = make_storage(monkeypatch, tmp_path, {"region": "eu-west-1"})
    client = storage.conn[0]
    try:
        with Stubber(client) as stub:
            stub.add_response("head_bucket", {}, {"Bucket": "knowledge-base"})
            stub.add_response("put_object", {}, {"Bucket": "knowledge-base", "Key": "file.txt", "Body": ANY, "ChecksumAlgorithm": ANY})

            storage.put("knowledge-base", "file.txt", b"document contents")

            stub.assert_no_pending_responses()
            storage.__open__.assert_not_called()
    finally:
        client.close()
