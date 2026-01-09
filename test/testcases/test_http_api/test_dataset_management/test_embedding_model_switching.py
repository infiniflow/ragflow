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
import pytest
from common import check_embedding, create_dataset, update_dataset, parse_documents


class TestCheckEmbedding:
    """Test cases for /check_embedding API endpoint"""

    @pytest.mark.p1
    def test_check_embedding_success(self, HttpApiAuth, add_document):
        """Test check_embedding with compatible same-dimension model"""
        dataset_id, document_id = add_document

        # Parse the document to create chunks
        res = parse_documents(HttpApiAuth, dataset_id, {"document_ids": [document_id]})
        assert res["code"] == 0, res

        # Check embedding with the same model (should succeed with high similarity)
        res = check_embedding(HttpApiAuth, {
            "kb_id": dataset_id,
            "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
            "check_num": 3
        })
        assert res["code"] == 0, res
        assert "summary" in res["data"], res
        assert res["data"]["summary"]["avg_cos_sim"] > 0.9, res

    @pytest.mark.p2
    def test_check_embedding_different_dimension(self, HttpApiAuth, add_document):
        """Test check_embedding with incompatible different-dimension model"""
        dataset_id, document_id = add_document

        # Parse the document to create chunks
        res = parse_documents(HttpApiAuth, dataset_id, {"document_ids": [document_id]})
        assert res["code"] == 0, res

        # Check embedding with a different dimension model
        # Note: This test assumes BAAI/bge-large-zh-v1.5 has different dimensions than bge-small-en-v1.5
        res = check_embedding(HttpApiAuth, {
            "kb_id": dataset_id,
            "embd_id": "BAAI/bge-large-zh-v1.5@Builtin",
            "check_num": 3
        })
        # Should return error due to dimension mismatch
        assert res["code"] == 10, res
        assert "dimension" in res["message"].lower(), res

    @pytest.mark.p2
    def test_check_embedding_empty_kb(self, HttpApiAuth, add_dataset):
        """Test check_embedding on empty knowledge base (no chunks)"""
        dataset_id = add_dataset

        res = check_embedding(HttpApiAuth, {
            "kb_id": dataset_id,
            "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
            "check_num": 3
        })
        # Should succeed or return appropriate message for empty KB
        assert res["code"] in [0, 10], res

    @pytest.mark.p3
    def test_check_embedding_invalid_kb_id(self, HttpApiAuth):
        """Test check_embedding with invalid knowledge base ID"""
        res = check_embedding(HttpApiAuth, {
            "kb_id": "invalid_kb_id",
            "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
            "check_num": 3
        })
        assert res["code"] != 0, res

    @pytest.mark.p3
    def test_check_embedding_invalid_model(self, HttpApiAuth, add_dataset):
        """Test check_embedding with invalid embedding model"""
        dataset_id = add_dataset

        res = check_embedding(HttpApiAuth, {
            "kb_id": dataset_id,
            "embd_id": "invalid_model@InvalidProvider",
            "check_num": 3
        })
        assert res["code"] != 0, res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "check_num, expected_message",
        [
            (-1, "Input should be greater than or equal to 1"),
            (0, "Input should be greater than or equal to 1"),
            (10001, "Input should be less than or equal to 10000"),
        ],
        ids=["negative", "zero", "too_large"]
    )
    def test_check_embedding_invalid_check_num(self, HttpApiAuth, add_dataset, check_num, expected_message):
        """Test check_embedding with invalid check_num parameter"""
        dataset_id = add_dataset

        res = check_embedding(HttpApiAuth, {
            "kb_id": dataset_id,
            "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
            "check_num": check_num
        })
        assert res["code"] == 101, res
        assert expected_message in res["message"], res


class TestEmbeddingModelSwitching:
    """Test cases for embedding model switching with dimension checks"""

    @pytest.mark.p1
    def test_switch_model_empty_kb(self, HttpApiAuth, add_dataset):
        """Test switching embedding model on empty knowledge base (should succeed)"""
        dataset_id = add_dataset

        # Switch to a different model on empty KB
        res = update_dataset(HttpApiAuth, dataset_id, {
            "embedding_model": "embedding-3@ZHIPU-AI"
        })
        assert res["code"] == 0, res
        assert res["data"]["embedding_model"] == "embedding-3@ZHIPU-AI", res

    @pytest.mark.p1
    def test_switch_model_with_chunks_same_dimension(self, HttpApiAuth, add_chunks):
        """Test switching to same-dimension model with chunks (should succeed if similarity > 0.9)"""
        dataset_id, document_id, chunk_ids = add_chunks

        # Switch to a compatible same-dimension model
        # Note: This assumes both models have same dimensions and are compatible
        res = update_dataset(HttpApiAuth, dataset_id, {
            "embedding_model": "BAAI/bge-small-en-v1.5@Builtin"
        })
        # Should succeed if dimensions match and similarity is high
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_switch_model_with_chunks_different_dimension(self, HttpApiAuth, add_chunks):
        """Test switching to different-dimension model with chunks (should fail)"""
        dataset_id, document_id, chunk_ids = add_chunks

        # Try to switch to a different dimension model
        res = update_dataset(HttpApiAuth, dataset_id, {
            "embedding_model": "BAAI/bge-large-zh-v1.5@Builtin"
        })
        # Should fail due to dimension mismatch
        assert res["code"] == 102, res
        assert "dimension" in res["message"].lower(), res

    @pytest.mark.p2
    def test_switch_model_to_same_model(self, HttpApiAuth, add_chunks):
        """Test switching to the same embedding model (should succeed)"""
        dataset_id, _, _ = add_chunks

        # Switch to the same model
        res = update_dataset(HttpApiAuth, dataset_id, {
            "embedding_model": "BAAI/bge-small-en-v1.5@Builtin"
        })
        assert res["code"] == 0, res
        assert res["data"]["embedding_model"] == "BAAI/bge-small-en-v1.5@Builtin", res

    @pytest.mark.p3
    def test_switch_model_invalid_format(self, HttpApiAuth, add_dataset):
        """Test switching with invalid model format"""
        dataset_id = add_dataset

        res = update_dataset(HttpApiAuth, dataset_id, {
            "embedding_model": "invalid_model_format"
        })
        assert res["code"] == 101, res
        assert "Embedding model identifier must follow" in res["message"], res

    @pytest.mark.p3
    def test_switch_model_unauthorized_model(self, HttpApiAuth, add_dataset):
        """Test switching to unauthorized model"""
        dataset_id = add_dataset

        res = update_dataset(HttpApiAuth, dataset_id, {
            "embedding_model": "text-embedding-3-small@OpenAI"
        })
        # Should fail due to unauthorized model
        assert res["code"] == 101, res
        assert "Unauthorized model" in res["message"] or "Unsupported model" in res["message"], res
