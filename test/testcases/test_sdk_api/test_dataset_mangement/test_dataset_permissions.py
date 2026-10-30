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

"""Check resource authorization with a valid credential from an unrelated tenant."""

import pytest
from ragflow_sdk import DataSet, RAGFlow

pytestmark = pytest.mark.integration


@pytest.mark.parametrize(
    "operation,message",
    [
        ("read", "Permission denied"),
        ("update", "Resource not found"),
        ("delete", "Permission denied"),
    ],
)
def test_other_tenant_cannot_access_private_dataset(client, dataset_factory, integration_settings, operation, message):
    key = integration_settings.get("other_api_key")
    if not key or key == integration_settings["api_key"]:
        pytest.fail("RAGFLOW_SDK_OTHER_API_KEY must belong to an unrelated test tenant")
    dataset = dataset_factory(permission="me")
    other = RAGFlow(key, integration_settings["base_url"])
    # Prove the second credential is authenticated; invalid tokens do not test permissions.
    assert dataset.id not in {item.id for item in other.list_datasets()}
    operations = {
        "read": lambda: other.list_datasets(id=dataset.id),
        "update": lambda: DataSet(other, {"id": dataset.id}).update({"name": "forbidden-rename"}),
        "delete": lambda: other.delete_datasets(ids=[dataset.id]),
    }
    with pytest.raises(Exception, match=f"^{message}$"):
        operations[operation]()
    persisted = client.list_datasets(id=dataset.id)
    assert len(persisted) == 1
    assert persisted[0].name == dataset.name
    assert persisted[0].permission == "me"
