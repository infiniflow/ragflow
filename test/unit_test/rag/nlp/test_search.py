
import unittest
from unittest.mock import MagicMock
from rag.nlp.search import Dealer

class TestChunkList(unittest.TestCase):
    def test_chunk_list_termination(self):
        # Mock dataStore
        mock_store = MagicMock()

        # Setup Dealer
        dealer = Dealer(mock_store)

        # Scenario:
        # Page 0: search returns hits, but get_fields returns empty (simulating filtering)
        # Page 1: search returns hits, get_fields returns chunks
        # Page 2: search returns empty (end of results)

        # Mock search results
        # We simulate the structure that might be returned by ES or handled by get_doc_ids
        res_page_0 = {"hits": {"hits": [{"_id": "1"}]}}
        res_page_1 = {"hits": {"hits": [{"_id": "2"}]}}
        res_page_2 = {"hits": {"hits": []}}

        mock_store.search.side_effect = [res_page_0, res_page_1, res_page_2]

        # Mock get_fields
        # Page 0: returns empty dict (filtered out)
        # Page 1: returns some content
        mock_store.get_fields.side_effect = [
            {},
            {"2": {"content_with_weight": "some content"}},
            {}
        ]

        # Mock get_doc_ids to behave consistently with search results
        # This will be used by the fix
        def get_doc_ids_side_effect(res):
            return [d["_id"] for d in res["hits"]["hits"]]

        mock_store.get_doc_ids.side_effect = get_doc_ids_side_effect

        # Run chunk_list
        # We simulate a max_count that covers multiple pages (bs=128)
        # We request 500 items, which is enough for multiple pages
        chunks = dealer.chunk_list("doc_id", "tenant_id", ["kb_id"], max_count=500)

        # With FIXED code:
        # Page 0: get_fields returns {}, but search had hits. Loop continues.
        # Page 1: get_fields returns {"2": ...}. Added to res.
        # Page 2: search has no hits. Loop breaks.
        # chunks will have 1 item.

        self.assertEqual(len(chunks), 1, "Should have retrieved chunks from the second page")

if __name__ == '__main__':
    unittest.main()
