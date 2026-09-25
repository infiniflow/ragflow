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
"""
Unit test for ``DocumentService.remove_wiki_products`` in
``api/db/services/document_service.py``.

The document store's ``search`` takes the dataset identifiers as a list and
builds one table name per entry. Handed a bare string it iterates the
characters, tries one table per character, finds none, logs "No valid tables
found" and returns nothing. A document removal then logs one error per
document in every dataset whose table exists, and the page-source tracking
rows of the removed document are never cleaned. Every search the cleanup
issues has to hand the store a list.
"""

import types
import unittest
from unittest.mock import patch

from api.db.services.document_service import DocumentService
from common import settings


class _RecordingStore:
    """Stands in for the document store: the table exists, every search finds nothing."""

    def __init__(self):
        self.searches = []

    def index_exist(self, index_name, dataset_id):
        return True

    def search(
        self,
        select_fields,
        highlight_fields,
        condition,
        match_expressions,
        order_by,
        offset,
        limit,
        index_names,
        knowledgebase_ids,
        *args,
        **kwargs,
    ):
        self.searches.append(knowledgebase_ids)
        return None

    def get_fields(self, res, fields):
        return {}

    def delete(self, condition, index_name, dataset_id):
        return 0

    def update(self, condition, new_value, index_name, dataset_id):
        return True


class RemoveWikiProductsDatasetArgumentTest(unittest.TestCase):
    def test_every_search_hands_the_store_a_list_of_dataset_ids(self):
        store = _RecordingStore()
        doc = types.SimpleNamespace(id="doc-1", kb_id="kb-1")
        with patch.object(settings, "docStoreConn", store):
            DocumentService.remove_wiki_products(doc, "tenant-1")
        self.assertTrue(store.searches, "the cleanup issued no search at all")
        for ids in store.searches:
            self.assertIsInstance(
                ids,
                list,
                f"the store was handed {ids!r} where a list of dataset ids is expected",
            )
            self.assertEqual(ids, ["kb-1"])


if __name__ == "__main__":
    unittest.main()
