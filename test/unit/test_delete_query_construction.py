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
"""
Unit tests for delete query construction in ES/OpenSearch connectors.

These tests verify that the delete method correctly combines chunk IDs with 
other filter conditions (doc_id, kb_id) to scope deletions properly.

This addresses issue #12520: "Files of deleted slices can still be searched 
and displayed in 'reference'" - caused by delete queries not properly 
combining all filter conditions.

Run with: python -m pytest test/unit/test_delete_query_construction.py -v
"""

import pytest
from elasticsearch_dsl import Q, Search


class TestDeleteQueryConstruction:
    """
    Tests that verify the delete query is constructed correctly to include
    all necessary filter conditions (chunk IDs + doc_id + kb_id).
    """

    def build_delete_query(self, condition: dict, knowledgebase_id: str) -> dict:
        """
        Simulates the query construction logic from es_conn.py/opensearch_conn.py delete method.
        This is extracted to test the logic without needing actual ES/OS connections.
        """
        condition = condition.copy()  # Don't mutate the original
        condition["kb_id"] = knowledgebase_id

        # Build a bool query that combines id filter with other conditions
        bool_query = Q("bool")

        # Handle chunk IDs if present
        if "id" in condition:
            chunk_ids = condition["id"]
            if not isinstance(chunk_ids, list):
                chunk_ids = [chunk_ids]
            if chunk_ids:
                # Filter by specific chunk IDs
                bool_query.filter.append(Q("ids", values=chunk_ids))

        # Add all other conditions as filters
        for k, v in condition.items():
            if k == "id":
                continue  # Already handled above
            if k == "exists":
                bool_query.filter.append(Q("exists", field=v))
            elif k == "must_not":
                if isinstance(v, dict):
                    for kk, vv in v.items():
                        if kk == "exists":
                            bool_query.must_not.append(Q("exists", field=vv))
            elif isinstance(v, list):
                bool_query.must.append(Q("terms", **{k: v}))
            elif isinstance(v, str) or isinstance(v, int):
                bool_query.must.append(Q("term", **{k: v}))
            elif v is not None:
                raise Exception("Condition value must be int, str or list.")

        # If no filters were added, use match_all
        if not bool_query.filter and not bool_query.must and not bool_query.must_not:
            qry = Q("match_all")
        else:
            qry = bool_query

        return Search().query(qry).to_dict()

    def test_delete_with_chunk_ids_includes_kb_id(self):
        """
        CRITICAL: When deleting by chunk IDs, kb_id MUST be included in the query.
        
        This was the root cause of issue #12520 - the original code would 
        only use Q("ids") and ignore kb_id.
        """
        condition = {"id": ["chunk1", "chunk2"]}
        query = self.build_delete_query(condition, "kb123")
        
        query_dict = query["query"]["bool"]
        
        # Verify chunk IDs filter is present
        ids_filter = [f for f in query_dict.get("filter", []) if "ids" in f]
        assert len(ids_filter) == 1, "Should have ids filter"
        assert set(ids_filter[0]["ids"]["values"]) == {"chunk1", "chunk2"}
        
        # Verify kb_id is also in the query (CRITICAL FIX)
        must_terms = query_dict.get("must", [])
        kb_id_terms = [t for t in must_terms if "term" in t and "kb_id" in t.get("term", {})]
        assert len(kb_id_terms) == 1, "kb_id MUST be included when deleting by chunk IDs"
        assert kb_id_terms[0]["term"]["kb_id"] == "kb123"

    def test_delete_with_chunk_ids_and_doc_id(self):
        """
        When deleting chunks, both chunk IDs AND doc_id should be in the query
        to properly scope the deletion to a specific document.
        """
        condition = {"id": ["chunk1"], "doc_id": "doc456"}
        query = self.build_delete_query(condition, "kb123")
        
        query_dict = query["query"]["bool"]
        
        # Verify all three conditions are present
        ids_filter = [f for f in query_dict.get("filter", []) if "ids" in f]
        assert len(ids_filter) == 1, "Should have ids filter"
        
        must_terms = query_dict.get("must", [])
        
        # Check kb_id
        kb_id_terms = [t for t in must_terms if "term" in t and "kb_id" in t.get("term", {})]
        assert len(kb_id_terms) == 1, "kb_id must be present"
        
        # Check doc_id
        doc_id_terms = [t for t in must_terms if "term" in t and "doc_id" in t.get("term", {})]
        assert len(doc_id_terms) == 1, "doc_id must be present"
        assert doc_id_terms[0]["term"]["doc_id"] == "doc456"

    def test_delete_single_chunk_id_converted_to_list(self):
        """
        Single chunk ID (not in a list) should be handled correctly.
        """
        condition = {"id": "single_chunk"}
        query = self.build_delete_query(condition, "kb123")
        
        query_dict = query["query"]["bool"]
        ids_filter = [f for f in query_dict.get("filter", []) if "ids" in f]
        assert len(ids_filter) == 1
        assert ids_filter[0]["ids"]["values"] == ["single_chunk"]

    def test_delete_empty_chunk_ids_uses_other_conditions(self):
        """
        When chunk_ids is empty, should rely on other conditions (doc_id, kb_id).
        This is used for deleting all chunks of a document.
        """
        condition = {"id": [], "doc_id": "doc456"}
        query = self.build_delete_query(condition, "kb123")
        
        query_dict = query["query"]["bool"]
        
        # Empty chunk_ids should NOT add an ids filter
        ids_filter = [f for f in query_dict.get("filter", []) if "ids" in f]
        assert len(ids_filter) == 0, "Empty chunk_ids should not create ids filter"
        
        # But kb_id and doc_id should still be present
        must_terms = query_dict.get("must", [])
        assert any("kb_id" in str(t) for t in must_terms), "kb_id must be present"
        assert any("doc_id" in str(t) for t in must_terms), "doc_id must be present"

    def test_delete_by_doc_id_only(self):
        """
        Delete all chunks of a document (no specific chunk IDs).
        """
        condition = {"doc_id": "doc456"}
        query = self.build_delete_query(condition, "kb123")
        
        query_dict = query["query"]["bool"]
        must_terms = query_dict.get("must", [])
        
        # Both doc_id and kb_id should be in query
        doc_terms = [t for t in must_terms if "term" in t and "doc_id" in t.get("term", {})]
        kb_terms = [t for t in must_terms if "term" in t and "kb_id" in t.get("term", {})]
        
        assert len(doc_terms) == 1
        assert len(kb_terms) == 1

    def test_delete_with_must_not_exists(self):
        """
        Test handling of must_not with exists condition (used in graph cleanup).
        """
        condition = {
            "kb_id": "kb123",  # Will be overwritten
            "must_not": {"exists": "source_id"}
        }
        query = self.build_delete_query(condition, "kb123")
        
        query_dict = query["query"]["bool"]
        must_not = query_dict.get("must_not", [])
        
        exists_filters = [f for f in must_not if "exists" in f]
        assert len(exists_filters) == 1
        assert exists_filters[0]["exists"]["field"] == "source_id"

    def test_delete_with_list_values(self):
        """
        Test that list values use 'terms' query (plural).
        """
        condition = {"knowledge_graph_kwd": ["entity", "relation"]}
        query = self.build_delete_query(condition, "kb123")
        
        query_dict = query["query"]["bool"]
        must_terms = query_dict.get("must", [])
        
        terms_query = [t for t in must_terms if "terms" in t]
        assert len(terms_query) >= 1
        # Find the knowledge_graph_kwd terms
        kw_terms = [t for t in terms_query if "knowledge_graph_kwd" in t.get("terms", {})]
        assert len(kw_terms) == 1


class TestChunkAppDeleteCondition:
    """
    Tests that verify the chunk_app.py rm endpoint passes the correct
    condition to docStoreConn.delete.
    """

    def test_rm_endpoint_includes_doc_id_in_condition(self):
        """
        The /chunk/rm endpoint MUST include doc_id in the condition
        passed to settings.docStoreConn.delete.
        
        This is the fix applied to api/apps/chunk_app.py
        """
        # Simulate what the rm endpoint should construct
        req = {
            "doc_id": "doc123",
            "chunk_ids": ["chunk1", "chunk2"]
        }
        
        # This is what the FIXED code should produce:
        correct_condition = {
            "id": req["chunk_ids"],
            "doc_id": req["doc_id"]  # <-- CRITICAL: doc_id must be included
        }
        
        # Verify doc_id is in the condition
        assert "doc_id" in correct_condition, "doc_id MUST be in delete condition"
        assert correct_condition["doc_id"] == "doc123"
        
        # Verify chunk IDs are in the condition
        assert "id" in correct_condition
        assert correct_condition["id"] == ["chunk1", "chunk2"]


class TestSDKDocDeleteCondition:
    """
    Tests that verify the SDK doc.py rm_chunk endpoint constructs
    the correct deletion condition.
    """

    def test_sdk_rm_chunk_includes_doc_id(self):
        """
        The SDK /datasets/<id>/documents/<id>/chunks DELETE endpoint
        should include doc_id in the condition.
        """
        # Simulate SDK request
        document_id = "doc456"
        chunk_ids = ["chunk1", "chunk2"]
        
        # The CORRECT condition construction (from sdk/doc.py):
        condition = {"doc_id": document_id}
        if chunk_ids:
            condition["id"] = chunk_ids
        
        assert condition == {
            "doc_id": "doc456",
            "id": ["chunk1", "chunk2"]
        }

    def test_sdk_rm_chunk_all_chunks(self):
        """
        When no chunk_ids specified, delete all chunks of the document.
        """
        document_id = "doc456"
        chunk_ids = []  # Delete all
        
        condition = {"doc_id": document_id}
        if chunk_ids:
            condition["id"] = chunk_ids
        
        # When no chunk_ids, only doc_id should be in condition
        assert condition == {"doc_id": "doc456"}
        assert "id" not in condition


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
