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
from common import (
    list_tags,
    aggregate_tags,
    delete_tags,
    rename_tag,
)


@pytest.mark.usefixtures("clear_datasets")
class TestListTags:
    @pytest.mark.p2
    def test_list_tags_success(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = list_tags(HttpApiAuth, dataset_id)
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_list_tags_invalid_id(self, HttpApiAuth):
        res = list_tags(HttpApiAuth, "invalid_id")
        assert res["code"] != 0, res


@pytest.mark.usefixtures("clear_datasets")
class TestAggregateTags:
    @pytest.mark.p2
    def test_aggregate_tags_success(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = aggregate_tags(HttpApiAuth, [dataset_id])
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_aggregate_tags_multiple_datasets(self, HttpApiAuth, add_datasets_func):
        dataset_ids = add_datasets_func
        res = aggregate_tags(HttpApiAuth, dataset_ids)
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_aggregate_tags_empty_ids(self, HttpApiAuth):
        res = aggregate_tags(HttpApiAuth, [])
        assert res["code"] != 0, res


@pytest.mark.usefixtures("clear_datasets")
class TestDeleteTags:
    @pytest.mark.p2
    def test_delete_tags_missing_body(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = delete_tags(HttpApiAuth, dataset_id, [])
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_delete_tags_invalid_id(self, HttpApiAuth):
        res = delete_tags(HttpApiAuth, "invalid_id", ["tag1"])
        assert res["code"] != 0, res


@pytest.mark.usefixtures("clear_datasets")
class TestRenameTag:
    @pytest.mark.p2
    def test_rename_tag_empty_names(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = rename_tag(HttpApiAuth, dataset_id, "", "")
        assert res["code"] != 0, res

    @pytest.mark.p2
    def test_rename_tag_invalid_id(self, HttpApiAuth):
        res = rename_tag(HttpApiAuth, "invalid_id", "old_tag", "new_tag")
        assert res["code"] != 0, res
