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

from api.db.services.connector_service import connector_doc_id_candidates, resolve_connector_doc_id
from api.utils.common import hash128

CONNECTOR_ID = "conn-1"
EXTERNAL_ID = "JIRA-42"
KB_A = "kb-a"
KB_B = "kb-b"

V0_ID = hash128(EXTERNAL_ID)
LEGACY_ID = hash128(f"{CONNECTOR_ID}:{EXTERNAL_ID}")
SCOPED_A = hash128(f"{KB_A}:{CONNECTOR_ID}:{EXTERNAL_ID}")
SCOPED_B = hash128(f"{KB_B}:{CONNECTOR_ID}:{EXTERNAL_ID}")


@pytest.mark.p2
def test_candidates_cover_every_historical_scheme():
    assert connector_doc_id_candidates(KB_A, CONNECTOR_ID, EXTERNAL_ID) == (V0_ID, LEGACY_ID, SCOPED_A)


@pytest.mark.p2
def test_first_sync_uses_kb_scoped_id():
    assert resolve_connector_doc_id(KB_A, CONNECTOR_ID, EXTERNAL_ID, set()) == SCOPED_A


@pytest.mark.p2
@pytest.mark.parametrize("owned_id", [LEGACY_ID, V0_ID])
def test_kb_agnostic_id_is_reused_when_this_kb_owns_it(owned_id):
    """Re-syncing an upgraded instance updates its rows instead of duplicating them."""
    assert resolve_connector_doc_id(KB_A, CONNECTOR_ID, EXTERNAL_ID, {owned_id}) == owned_id


@pytest.mark.p2
@pytest.mark.parametrize("foreign_id", [LEGACY_ID, V0_ID])
def test_kb_agnostic_id_owned_elsewhere_is_never_reused(foreign_id):
    """Regression test for #18116.

    ``owned_doc_ids`` lists only what this knowledge base already holds, so an
    id absent from it may well exist under a different knowledge base. Handing
    that id to ``FileService.upload_document`` is what produced "Existing
    document id collision with another knowledge base; skipping update." and
    dropped the document. KB B must route around it.
    """
    kb_a_owns = {foreign_id}
    kb_b_owns = set()

    assert resolve_connector_doc_id(KB_A, CONNECTOR_ID, EXTERNAL_ID, kb_a_owns) == foreign_id
    assert resolve_connector_doc_id(KB_B, CONNECTOR_ID, EXTERNAL_ID, kb_b_owns) == SCOPED_B
    assert SCOPED_B != foreign_id


@pytest.mark.p2
def test_oldest_owned_id_wins_when_several_upgrades_left_rows_behind():
    """An instance carrying rows from more than one upgrade settles, not drifts.

    Preferring the newest owned id would re-key the document on every upgrade
    and strand the older row again each time.
    """
    assert resolve_connector_doc_id(KB_A, CONNECTOR_ID, EXTERNAL_ID, {V0_ID, LEGACY_ID}) == V0_ID


@pytest.mark.p2
def test_two_kbs_on_one_connector_never_share_an_id():
    assert resolve_connector_doc_id(KB_A, CONNECTOR_ID, EXTERNAL_ID, set()) != resolve_connector_doc_id(KB_B, CONNECTOR_ID, EXTERNAL_ID, set())


@pytest.mark.p2
def test_resolution_is_stable_across_runs():
    """The second run sees the id the first run wrote and settles on it."""
    first = resolve_connector_doc_id(KB_A, CONNECTOR_ID, EXTERNAL_ID, set())
    second = resolve_connector_doc_id(KB_A, CONNECTOR_ID, EXTERNAL_ID, {first})
    assert first == second == SCOPED_A
