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

"""Pre-register the ``common.data_source`` package namespace so that
importing individual sub-modules (config, exceptions, rest_api_connector, …)
does **not** trigger ``common/data_source/__init__.py``, which pulls in every
connector and their heavy transitive dependencies (numpy, xgboost, etc.).

This file is executed by pytest before any test module in this directory is
collected, so the lightweight namespace is always in place.
"""

import os
import sys
import types

import common  # lightweight top-level package

if "common.data_source" not in sys.modules:
    _pkg = types.ModuleType("common.data_source")
    _pkg.__path__ = [os.path.join(p, "data_source") for p in common.__path__]
    _pkg.__package__ = "common.data_source"
    sys.modules["common.data_source"] = _pkg
