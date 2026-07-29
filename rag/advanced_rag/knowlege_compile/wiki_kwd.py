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

"""Wiki-compilation ``compile_kwd`` values — single source of truth.

The feature's stored row types were renamed ``artifact_* -> wiki_*``. These
values are **persisted** in the doc store (the ``compile_kwd`` field on every
wiki row), so a straight rename would make already-compiled datasets invisible.
During the migration window:

* **writes** use only the new ``wiki_*`` value (the constants below), while
* **reads / deletes** must accept BOTH the new value and its legacy ``artifact_*``
  alias — use :func:`read_kwds` to expand a condition's ``compile_kwd`` list.

Lives in ``knowlege_compile`` (the lower layer) so both ``wiki.py`` and
``rag.svr.task_executor_refactor.dataset_wiki_generator`` — which import each
other-adjacent modules — can share one definition without a circular import.
Once the Phase-3 data migration rewrites every legacy row, the aliases and
``read_kwds`` can be dropped.
"""

WIKI_MAP_COMPILE_KWD = "wiki_map_extract"
WIKI_REDUCE_COMPILE_KWD = "wiki_reduce_result"
WIKI_PLAN_COMPILE_KWD = "wiki_compilation_plan"
WIKI_DRAFT_COMPILE_KWD = "wiki_page_draft"
WIKI_PAGE_COMPILE_KWD = "wiki_page"
WIKI_PAGE_TOPIC_COMPILE_KWD = "wiki_page_topic"
WIKI_ENTITY_COMPILE_KWD = "wiki_entity"
WIKI_RELATION_COMPILE_KWD = "wiki_relation"
WIKI_PAGE_GRAPH_COMPILE_KWD = "wiki_page_graph"

# Every derived (non-MAP) wiki row type. ``artifact_map_extract`` (per-doc resume
# rows) is handled separately by the callers, so it is intentionally excluded.
WIKI_DERIVED_COMPILE_KWDS = (
    WIKI_REDUCE_COMPILE_KWD,
    WIKI_PLAN_COMPILE_KWD,
    WIKI_DRAFT_COMPILE_KWD,
    WIKI_PAGE_COMPILE_KWD,
    WIKI_PAGE_TOPIC_COMPILE_KWD,
    WIKI_ENTITY_COMPILE_KWD,
    WIKI_RELATION_COMPILE_KWD,
    WIKI_PAGE_GRAPH_COMPILE_KWD,
)

# All wiki row types (MAP + derived) — used by clear-all / delete paths.
WIKI_ALL_COMPILE_KWDS = (WIKI_MAP_COMPILE_KWD,) + WIKI_DERIVED_COMPILE_KWDS

# new ``wiki_*`` value -> legacy ``artifact_*`` value still present in datasets
# compiled before the rename. Reads and deletes must cover both.
_LEGACY_COMPILE_KWD = {
    WIKI_MAP_COMPILE_KWD: "artifact_map_extract",
    WIKI_REDUCE_COMPILE_KWD: "artifact_reduce_result",
    WIKI_PLAN_COMPILE_KWD: "artifact_compilation_plan",
    WIKI_DRAFT_COMPILE_KWD: "artifact_page_draft",
    WIKI_PAGE_COMPILE_KWD: "artifact_page",
    WIKI_PAGE_TOPIC_COMPILE_KWD: "artifact_page_topic",
    WIKI_ENTITY_COMPILE_KWD: "artifact_entity",
    WIKI_RELATION_COMPILE_KWD: "artifact_relation",
    WIKI_PAGE_GRAPH_COMPILE_KWD: "artifact_page_graph",
}


def read_kwds(*kwds: str) -> list[str]:
    """Expand compile_kwd value(s) to ``[new, legacy, ...]`` for dual-read /
    dual-delete conditions. Order-preserving, deduped. Values without a known
    legacy alias pass through unchanged."""
    out: list[str] = []
    for kwd in kwds:
        if kwd and kwd not in out:
            out.append(kwd)
        legacy = _LEGACY_COMPILE_KWD.get(kwd)
        if legacy and legacy not in out:
            out.append(legacy)
    return out


def legacy_kwd(kwd: str) -> str | None:
    """The legacy ``artifact_*`` alias for a new ``wiki_*`` value, or None."""
    return _LEGACY_COMPILE_KWD.get(kwd)
