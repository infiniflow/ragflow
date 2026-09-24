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

"""File list ordering validation against the database model."""

import pytest
from peewee import Ordering
from pydantic import ValidationError

from api.db.db_models import File
from api.utils.validation_utils import ListFileReq


@pytest.mark.parametrize("orderby", ["bogus", "", "save"])
@pytest.mark.parametrize("desc", [True, False])
def test_unknown_orderby_builds_file_ordering(orderby, desc):
    request = ListFileReq(orderby=orderby, desc=desc)
    field = File.getter_by(request.orderby)
    ordering = field.desc() if request.desc else field.asc()

    assert isinstance(ordering, Ordering)
    assert request.desc is desc
    assert ordering.direction == ("DESC" if desc else "ASC")


@pytest.mark.parametrize(
    "orderby",
    [
        "id",
        "parent_id",
        "tenant_id",
        "created_by",
        "name",
        "location",
        "size",
        "type",
        "source_type",
        "create_time",
        "create_date",
        "update_time",
        "update_date",
    ],
)
@pytest.mark.parametrize("desc", [True, False])
def test_file_column_and_direction_are_preserved(orderby, desc):
    request = ListFileReq(orderby=orderby, desc=desc)
    field = File.getter_by(request.orderby)
    ordering = field.desc() if request.desc else field.asc()

    assert request.orderby == orderby
    assert request.desc is desc
    assert isinstance(ordering, Ordering)
    assert ordering.node is File._meta.fields[orderby]
    assert ordering.direction == ("DESC" if desc else "ASC")


@pytest.mark.parametrize("orderby", ["bogus", "", "save"])
def test_unknown_orderby_falls_back_to_create_time(orderby):
    assert ListFileReq(orderby=orderby).orderby == "create_time"


@pytest.mark.parametrize("orderby", [None, 1, True, [], {}])
def test_non_string_orderby_remains_invalid(orderby):
    with pytest.raises(ValidationError):
        ListFileReq(orderby=orderby)
