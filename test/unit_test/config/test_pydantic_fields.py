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
from pydantic import BaseModel, ValidationError


class M(BaseModel):
    flag: bool


@pytest.mark.parametrize("value", [
    "1",
    "true",
    "True",
    "TRUE",
    "yes",
    "on",
    1,
    True,
])
def test_bool_true(value):
    m = M(flag=value)
    assert m.flag is True


@pytest.mark.parametrize("value", [
    "0",
    "false",
    "False",
    "FALSE",
    "no",
    "off",
    0,
    False,
])
def test_bool_false(value):
    m = M(flag=value)
    assert m.flag is False


@pytest.mark.parametrize("value", [
    "",
    "abc",
    "enabled",
    "disabled",
    None,
])
def test_bool_invalid(value):
    with pytest.raises(ValidationError):
        M(flag=value)


def test_pydantic_bool_autocast():
    assert M(flag="true").flag is True
    assert M(flag="false").flag is False
    assert M(flag="1").flag is True
    assert M(flag="0").flag is False
