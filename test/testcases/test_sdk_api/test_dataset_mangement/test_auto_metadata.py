#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#  http://www.apache.org/licenses/LICENSE-2.0
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

import pytest


@pytest.mark.usefixtures("clear_datasets")
class TestAutoMetadataOnCreate:
    @pytest.mark.p1
    def test_create_dataset_with_auto_metadata(self, client):
        payload = {
            "name": "auto_metadata_create",
            "auto_metadata_config": {
                "enabled": True,
                "fields": [
                    {
                        "name": "author",
                        "type": "string",
                        "description": "The author of the document",
                        "examples": ["John Doe", "Jane Smith"],
                        "restrict_values": False,
                    },
                    {
                        "name": "category",
                        "type": "list",
                        "description": "Document category",
                        "examples": ["Technical", "Business"],
                        "restrict_values": True,
                    },
                ],
            },
        }
        dataset = client.create_dataset(**payload)
        # The SDK should expose parser_config via internal properties or metadata;
        # we rely on the HTTP API for verification via get_auto_metadata.
        cfg = client.get_auto_metadata(dataset_id=dataset.id)
        assert cfg["enabled"] is True
        assert len(cfg["fields"]) == 2
        names = {f["name"] for f in cfg["fields"]}
        assert names == {"author", "category"}


@pytest.mark.usefixtures("clear_datasets")
class TestAutoMetadataOnUpdate:
    @pytest.mark.p1
    def test_update_auto_metadata_via_dataset_update(self, client, add_dataset_func):
        dataset = add_dataset_func

        # Initially set auto-metadata via dataset.update
        payload = {
            "auto_metadata_config": {
                "enabled": True,
                "fields": [
                    {
                        "name": "tags",
                        "type": "list",
                        "description": "Document tags",
                        "examples": ["AI", "ML", "RAG"],
                        "restrict_values": False,
                    }
                ],
            }
        }
        dataset.update(payload)

        cfg = client.get_auto_metadata(dataset_id=dataset.id)
        assert cfg["enabled"] is True
        assert len(cfg["fields"]) == 1
        assert cfg["fields"][0]["name"] == "tags"
        assert cfg["fields"][0]["type"] == "list"

        # Disable auto-metadata and replace fields
        update_cfg = {
            "enabled": False,
            "fields": [
                {
                    "name": "year",
                    "type": "time",
                    "description": "Publication year",
                    "examples": None,
                    "restrict_values": False,
                }
            ],
        }
        client.update_auto_metadata(dataset_id=dataset.id, **update_cfg)

        cfg2 = client.get_auto_metadata(dataset_id=dataset.id)
        assert cfg2["enabled"] is False
        assert len(cfg2["fields"]) == 1
        assert cfg2["fields"][0]["name"] == "year"
        assert cfg2["fields"][0]["type"] == "time"


@pytest.mark.usefixtures("clear_datasets")
class TestAutoMetadataValidation:
    @pytest.mark.p2
    def test_invalid_field_type_rejected(self, client):
        payload = {
            "name": "auto_metadata_invalid_type",
            "auto_metadata_config": {
                "enabled": True,
                "fields": [
                    {
                        "name": "invalid_type",
                        "type": "unknown",  # invalid literal
                    }
                ],
            },
        }
        with pytest.raises(Exception) as exc_info:
            client.create_dataset(**payload)
        msg = str(exc_info.value)
        # Pydantic literal_error message should appear
        assert "Input should be" in msg or "literal_error" in msg

