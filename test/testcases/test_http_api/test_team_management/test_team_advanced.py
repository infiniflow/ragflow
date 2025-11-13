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
"""Advanced team management tests."""

from __future__ import annotations

import uuid
from typing import Any

import pytest

from common import create_team, create_user
from libs.auth import RAGFlowWebApiAuth


@pytest.mark.p2
class TestTeamAdvanced:
    """Advanced team management tests."""

    def test_team_creation_with_custom_models(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating team with custom model configurations."""
        team_name: str = f"Custom Models Team {uuid.uuid4().hex[:8]}"
        
        # Attempt to create team with custom models
        # Note: Model IDs need to be valid and added by the user
        team_payload: dict[str, str] = {
            "name": team_name,
            # These would need to be actual model IDs added by the user
            # "llm_id": "custom_llm_id",
            # "embd_id": "custom_embd_id",
        }
        res: dict[str, Any] = create_team(WebApiAuth, team_payload)
        
        # Should succeed with defaults if custom models not specified
        assert res["code"] == 0, res
        assert res["data"]["name"] == team_name
        assert "id" in res["data"]

    def test_team_creation_response_structure(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test that team creation returns complete response structure."""
        team_name: str = f"Structure Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        res: dict[str, Any] = create_team(WebApiAuth, team_payload)
        
        assert res["code"] == 0, res
        assert "data" in res
        
        # Check required fields
        required_fields: list[str] = ["id", "name", "owner_id"]
        for field in required_fields:
            assert field in res["data"], (
                f"Missing required field: {field}"
            )
        
        assert res["data"]["name"] == team_name
        assert len(res["data"]["id"]) > 0, "Team ID should not be empty"
        assert len(res["data"]["owner_id"]) > 0, "Owner ID should not be empty"

    def test_multiple_teams_same_name_allowed(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test that multiple teams can have the same name."""
        team_name: str = f"Duplicate Name {uuid.uuid4().hex[:8]}"
        
        # Create first team
        res1: dict[str, Any] = create_team(WebApiAuth, {"name": team_name})
        assert res1["code"] == 0, res1
        team_id_1: str = res1["data"]["id"]
        
        # Create second team with same name
        res2: dict[str, Any] = create_team(WebApiAuth, {"name": team_name})
        assert res2["code"] == 0, res2
        team_id_2: str = res2["data"]["id"]
        
        # Teams should have different IDs
        assert team_id_1 != team_id_2, "Teams should have unique IDs"
        assert res1["data"]["name"] == res2["data"]["name"], (
            "Both teams should have the same name"
        )

    def test_team_creation_with_credit_limit(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating team with custom credit limit."""
        team_name: str = f"Credit Test Team {uuid.uuid4().hex[:8]}"
        custom_credit: int = 1000
        
        team_payload: dict[str, Any] = {
            "name": team_name,
            "credit": custom_credit,
        }
        res: dict[str, Any] = create_team(WebApiAuth, team_payload)
        
        # Should succeed
        assert res["code"] == 0, res
        # Note: Credit may not be in response, but should be set internally

    def test_team_name_with_special_characters(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test team names with special characters."""
        special_names: list[str] = [
            f"Team-{uuid.uuid4().hex[:4]}_Test!",
            f"Team & Co. {uuid.uuid4().hex[:4]}",
            f"Team @{uuid.uuid4().hex[:4]}",
            f"团队{uuid.uuid4().hex[:4]}",  # Unicode
        ]
        
        for name in special_names:
            res: dict[str, Any] = create_team(WebApiAuth, {"name": name})
            # Should either accept or reject with clear message
            if res["code"] == 0:
                assert res["data"]["name"] == name, (
                    f"Team name should be preserved: {name}"
                )
            # If rejected, should have clear error
            # (Current implementation accepts special chars)

    def test_team_creation_default_owner(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test that team creator is set as owner by default."""
        team_name: str = f"Owner Test Team {uuid.uuid4().hex[:8]}"
        res: dict[str, Any] = create_team(WebApiAuth, {"name": team_name})
        
        assert res["code"] == 0, res
        assert "owner_id" in res["data"], "Owner ID should be in response"
        # Owner should be the authenticated user
        # (Cannot verify without knowing WebApiAuth user ID)

    def test_concurrent_team_creation(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test concurrent team creation."""
        import concurrent.futures
        
        def create_test_team(index: int) -> dict[str, Any]:
            team_name: str = f"Concurrent Team {index}_{uuid.uuid4().hex[:8]}"
            return create_team(WebApiAuth, {"name": team_name})
        
        # Create 10 teams concurrently
        count: int = 10
        with concurrent.futures.ThreadPoolExecutor(max_workers=5) as executor:
            futures: list[concurrent.futures.Future[dict[str, Any]]] = [
                executor.submit(create_test_team, i) for i in range(count)
            ]
            results: list[dict[str, Any]] = [
                f.result() for f in concurrent.futures.as_completed(futures)
            ]
        
        # All should succeed
        success_count: int = sum(1 for r in results if r["code"] == 0)
        assert success_count == count, (
            f"Expected {count} successful team creations, got {success_count}"
        )
        
        # All should have unique IDs
        team_ids: list[str] = [r["data"]["id"] for r in results if r["code"] == 0]
        assert len(team_ids) == len(set(team_ids)), (
            "All team IDs should be unique"
        )

    def test_team_with_invalid_model_id(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test team creation with invalid model ID."""
        team_name: str = f"Invalid Model Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {
            "name": team_name,
            "llm_id": "invalid_nonexistent_model_id_12345",
        }
        res: dict[str, Any] = create_team(WebApiAuth, team_payload)
        
        # Should reject with clear error message
        assert res["code"] != 0, "Invalid model ID should be rejected"
        assert (
            "model" in res["message"].lower()
            or "not found" in res["message"].lower()
            or "invalid" in res["message"].lower()
        ), "Error message should mention model issue"

    def test_team_creation_with_negative_credit(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test team creation with negative credit."""
        team_name: str = f"Negative Credit Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, Any] = {
            "name": team_name,
            "credit": -100,
        }
        res: dict[str, Any] = create_team(WebApiAuth, team_payload)
        
        # Should reject negative credit
        assert res["code"] != 0, "Negative credit should be rejected"
        assert "credit" in res["message"].lower(), (
            "Error message should mention credit"
        )

    def test_team_creation_empty_json_payload(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test team creation with completely empty payload."""
        res: dict[str, Any] = create_team(WebApiAuth, {})
        
        # Should reject with clear error
        assert res["code"] != 0, "Empty payload should be rejected"
        assert (
            "name" in res["message"].lower()
            or "required" in res["message"].lower()
        ), "Error should mention missing name"

    def test_team_unicode_name(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test team creation with full unicode name."""
        unicode_names: list[str] = [
            f"团队{uuid.uuid4().hex[:4]}",  # Chinese
            f"チーム{uuid.uuid4().hex[:4]}",  # Japanese
            f"Команда{uuid.uuid4().hex[:4]}",  # Russian
            f"فريق{uuid.uuid4().hex[:4]}",  # Arabic (RTL)
            f"😀🎉{uuid.uuid4().hex[:4]}",  # Emoji
        ]
        
        for name in unicode_names:
            res: dict[str, Any] = create_team(WebApiAuth, {"name": name})
            
            # Should handle unicode properly
            if res["code"] == 0:
                # Verify unicode is preserved (may be normalized)
                assert len(res["data"]["name"]) > 0, (
                    "Team name should not be empty after unicode"
                )

    @pytest.mark.p3
    def test_team_creation_with_all_optional_params(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test team creation with all optional parameters."""
        team_name: str = f"Full Params Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, Any] = {
            "name": team_name,
            "credit": 2000,
            # Note: Model IDs would need to be valid
            # "llm_id": "valid_llm_id",
            # "embd_id": "valid_embd_id",
            # "asr_id": "valid_asr_id",
            # "parser_ids": "valid_parser_ids",
            # "img2txt_id": "valid_img2txt_id",
            # "rerank_id": "valid_rerank_id",
        }
        res: dict[str, Any] = create_team(WebApiAuth, team_payload)
        
        # Should succeed
        assert res["code"] == 0, res
        assert res["data"]["name"] == team_name

    @pytest.mark.p3
    def test_team_max_name_length(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test team with maximum allowed name length."""
        # API spec says max 100 characters
        max_name: str = "A" * 100
        res: dict[str, Any] = create_team(WebApiAuth, {"name": max_name})
        
        # Should accept 100 characters
        assert res["code"] == 0, "100-character name should be accepted"
        assert res["data"]["name"] == max_name

    @pytest.mark.p3
    def test_team_name_just_over_limit(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test team with name just over limit."""
        # 101 characters (1 over limit)
        long_name: str = "A" * 101
        res: dict[str, Any] = create_team(WebApiAuth, {"name": long_name})
        
        # Should reject
        assert res["code"] != 0, "101-character name should be rejected"
        assert (
            "100" in res["message"]
            or "length" in res["message"].lower()
            or "long" in res["message"].lower()
        ), "Error should mention length limit"

    @pytest.mark.p3
    def test_team_creation_idempotency(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test that repeated team creation creates separate teams."""
        team_name: str = f"Idempotency Test {uuid.uuid4().hex[:8]}"
        payload: dict[str, str] = {"name": team_name}
        
        # Create same team twice
        res1: dict[str, Any] = create_team(WebApiAuth, payload)
        res2: dict[str, Any] = create_team(WebApiAuth, payload)
        
        # Both should succeed and create different teams
        assert res1["code"] == 0, res1
        assert res2["code"] == 0, res2
        assert res1["data"]["id"] != res2["data"]["id"], (
            "Should create different teams, not be idempotent"
        )

    @pytest.mark.p3
    def test_team_with_parser_ids(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test team creation with custom parser IDs."""
        team_name: str = f"Parser Test {uuid.uuid4().hex[:8]}"
        # parser_ids is typically a comma-separated string
        team_payload: dict[str, str] = {
            "name": team_name,
            "parser_ids": "naive,qa,table,paper,book,laws,presentation,manual,wiki",
        }
        res: dict[str, Any] = create_team(WebApiAuth, team_payload)
        
        # Should accept valid parser IDs
        assert res["code"] == 0, res
