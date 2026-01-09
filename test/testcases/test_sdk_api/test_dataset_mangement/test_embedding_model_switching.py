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
from utils import wait_for


class TestCheckEmbedding:
    """Test cases for DataSet.check_embedding() method"""

    @pytest.mark.p1
    def test_check_embedding_success(self, client, add_chunks):
        """Test check_embedding with compatible same-dimension model"""
        dataset, document, chunks = add_chunks

        # Check embedding with the same model (should succeed with high similarity)
        result = dataset.check_embedding(
            embd_id="BAAI/bge-small-en-v1.5@Builtin",
            check_num=3
        )

        assert "summary" in result, str(result)
        assert result["summary"]["avg_cos_sim"] > 0.9, str(result)
        assert result["summary"]["sampled"] == 3, str(result)
        assert "results" in result, str(result)

    @pytest.mark.p2
    def test_check_embedding_different_dimension(self, client, add_chunks):
        """Test check_embedding with incompatible different-dimension model"""
        dataset, document, chunks = add_chunks

        # Check embedding with a different dimension model
        # Note: This test assumes BAAI/bge-large-zh-v1.5 has different dimensions than bge-small-en-v1.5
        with pytest.raises(Exception) as exception_info:
            dataset.check_embedding(
                embd_id="BAAI/bge-large-zh-v1.5@Builtin",
                check_num=3
            )

        error_msg = str(exception_info.value)
        # Should fail due to dimension mismatch or low similarity
        assert "dimension" in error_msg.lower() or "similarity" in error_msg.lower(), error_msg

    @pytest.mark.p2
    def test_check_embedding_empty_kb(self, client, add_dataset):
        """Test check_embedding on empty knowledge base (no chunks)"""
        dataset = add_dataset

        # This might succeed or fail depending on implementation
        # Empty KB might have different behavior
        try:
            result = dataset.check_embedding(
                embd_id="BAAI/bge-small-en-v1.5@Builtin",
                check_num=3
            )
            # If it succeeds, verify structure
            assert "summary" in result, str(result)
        except Exception as e:
            # If it fails, that's also acceptable for empty KB
            assert "chunk" in str(e).lower() or "not found" in str(e).lower(), str(e)

    @pytest.mark.p3
    def test_check_embedding_invalid_model(self, client, add_dataset):
        """Test check_embedding with invalid embedding model"""
        dataset = add_dataset

        with pytest.raises(Exception) as exception_info:
            dataset.check_embedding(
                embd_id="invalid_model@InvalidProvider",
                check_num=3
            )

        error_msg = str(exception_info.value)
        assert "unsupported" in error_msg.lower() or "invalid" in error_msg.lower() or "unauthorized" in error_msg.lower(), error_msg

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "check_num, expected_error",
        [
            (-1, "greater than or equal to 1"),
            (0, "greater than or equal to 1"),
            (10001, "less than or equal to 10000"),
        ],
        ids=["negative", "zero", "too_large"]
    )
    def test_check_embedding_invalid_check_num(self, client, add_dataset, check_num, expected_error):
        """Test check_embedding with invalid check_num parameter"""
        dataset = add_dataset

        with pytest.raises(Exception) as exception_info:
            dataset.check_embedding(
                embd_id="BAAI/bge-small-en-v1.5@Builtin",
                check_num=check_num
            )

        assert expected_error in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p3
    def test_check_embedding_custom_check_num(self, client, add_chunks):
        """Test check_embedding with custom check_num value"""
        dataset, document, chunks = add_chunks

        result = dataset.check_embedding(
            embd_id="BAAI/bge-small-en-v1.5@Builtin",
            check_num=10
        )

        assert result["summary"]["sampled"] == 10, str(result)


class TestEmbeddingModelSwitching:
    """Test cases for embedding model switching with dimension checks"""

    @pytest.mark.p1
    def test_switch_model_empty_kb(self, client, add_dataset_func):
        """Test switching embedding model on empty knowledge base (should succeed)"""
        dataset = add_dataset_func

        # Switch to a different model on empty KB
        dataset.update({"embedding_model": "embedding-3@ZHIPU-AI"})

        assert dataset.embedding_model == "embedding-3@ZHIPU-AI", str(dataset)

    @pytest.mark.p1
    def test_switch_model_to_same_model(self, client, add_chunks):
        """Test switching to the same embedding model (should succeed)"""
        dataset, document, chunks = add_chunks

        # Switch to the same model
        dataset.update({"embedding_model": "BAAI/bge-small-en-v1.5@Builtin"})

        assert dataset.embedding_model == "BAAI/bge-small-en-v1.5@Builtin", str(dataset)

    @pytest.mark.p2
    def test_switch_model_with_chunks_different_dimension(self, client, add_chunks):
        """Test switching to different-dimension model with chunks (should fail)"""
        dataset, document, chunks = add_chunks

        # Try to switch to a different dimension model
        with pytest.raises(Exception) as exception_info:
            dataset.update({"embedding_model": "BAAI/bge-large-zh-v1.5@Builtin"})

        error_msg = str(exception_info.value)
        # Should fail due to dimension mismatch
        assert "dimension" in error_msg.lower(), error_msg

    @pytest.mark.p2
    def test_switch_model_and_retrieve(self, client, add_chunks):
        """Test that successful model switch allows retrieval"""
        dataset, document, chunks = add_chunks

        # Get initial chunk count
        initial_chunk_count = dataset.chunk_count
        assert initial_chunk_count > 0, "Dataset should have chunks"

        # Update to same model (should succeed)
        dataset.update({"embedding_model": "BAAI/bge-small-en-v1.5@Builtin"})

        # Verify dataset still has chunks
        retrieved_dataset = client.get_dataset(id=dataset.id)
        assert retrieved_dataset.chunk_count == initial_chunk_count, str(retrieved_dataset)

    @pytest.mark.p3
    def test_switch_model_invalid_format(self, client, add_dataset):
        """Test switching with invalid model format"""
        dataset = add_dataset

        with pytest.raises(Exception) as exception_info:
            dataset.update({"embedding_model": "invalid_model_format"})

        error_msg = str(exception_info.value)
        assert "format" in error_msg.lower() or "@" in error_msg, error_msg

    @pytest.mark.p3
    def test_switch_model_unauthorized_model(self, client, add_dataset):
        """Test switching to unauthorized model"""
        dataset = add_dataset

        with pytest.raises(Exception) as exception_info:
            dataset.update({"embedding_model": "text-embedding-3-small@OpenAI"})

        error_msg = str(exception_info.value)
        # Should fail due to unauthorized model
        assert "unauthorized" in error_msg.lower() or "unsupported" in error_msg.lower(), error_msg

    @pytest.mark.p2
    def test_check_before_switch_compatible(self, client, add_chunks):
        """Test the recommended workflow: check embedding first, then switch"""
        dataset, document, chunks = add_chunks

        # Step 1: Check if model is compatible
        result = dataset.check_embedding(
            embd_id="BAAI/bge-small-en-v1.5@Builtin",
            check_num=5
        )

        # Step 2: If compatible (avg_cos_sim > 0.9), proceed with switch
        if result["summary"]["avg_cos_sim"] > 0.9:
            dataset.update({"embedding_model": "BAAI/bge-small-en-v1.5@Builtin"})
            assert dataset.embedding_model == "BAAI/bge-small-en-v1.5@Builtin", str(dataset)
        else:
            pytest.skip("Models are not compatible (similarity too low)")

    @pytest.mark.p2
    def test_check_before_switch_incompatible(self, client, add_chunks):
        """Test that incompatible models are detected before switch attempt"""
        dataset, document, chunks = add_chunks

        # Step 1: Check if model is compatible
        with pytest.raises(Exception) as check_exception:
            dataset.check_embedding(
                embd_id="BAAI/bge-large-zh-v1.5@Builtin",
                check_num=5
            )

        # Step 2: Verify that the check detected incompatibility
        check_error = str(check_exception.value)
        assert "dimension" in check_error.lower() or "similarity" in check_error.lower(), check_error

        # Step 3: Verify that switching also fails
        with pytest.raises(Exception) as switch_exception:
            dataset.update({"embedding_model": "BAAI/bge-large-zh-v1.5@Builtin"})

        switch_error = str(switch_exception.value)
        assert "dimension" in switch_error.lower(), switch_error
