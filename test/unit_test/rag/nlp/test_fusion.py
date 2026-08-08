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

from __future__ import annotations

import importlib.util
from pathlib import Path

import pytest


_FUSION_PATH = Path(__file__).parents[4] / "rag" / "nlp" / "fusion.py"
_SPEC = importlib.util.spec_from_file_location("ragflow_fusion", _FUSION_PATH)
_FUSION_MODULE = importlib.util.module_from_spec(_SPEC)
assert _SPEC.loader is not None
_SPEC.loader.exec_module(_FUSION_MODULE)
build_fusion_expr = _FUSION_MODULE.build_fusion_expr


@pytest.mark.parametrize(
    ("vector_similarity_weight", "expected_weights"),
    [
        (0.0, "1,0"),
        (0.3, "0.7,0.3"),
        (0.5, "0.5,0.5"),
        (1.0, "0,1"),
    ],
)
def test_build_fusion_expr_uses_vector_similarity_weight(vector_similarity_weight, expected_weights):
    fusion_expr = build_fusion_expr(topn=10, vector_similarity_weight=vector_similarity_weight)

    assert fusion_expr.method == "weighted_sum"
    assert fusion_expr.topn == 10
    assert fusion_expr.fusion_params["weights"] == expected_weights
