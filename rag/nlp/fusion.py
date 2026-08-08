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

from common.doc_store.doc_store_base import FusionExpr


def build_fusion_expr(topn: int, vector_similarity_weight: float = 0.3) -> FusionExpr:
    """Build the Infinity weighted-sum expression from the vector weight."""
    term_similarity_weight = 1 - vector_similarity_weight
    return FusionExpr(
        "weighted_sum",
        topn,
        {"weights": f"{term_similarity_weight:g},{vector_similarity_weight:g}"},
    )
