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
from __future__ import annotations

import uuid
from typing import Any

import pytest

from common import create_team
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior during team creation."""

    @pytest.mark.parametrize(
        ("invalid_auth", "expected_code", "expected_message"),
        [
            # Endpoint now requires @login_required (JWT token auth)
            (None, 401, "Unauthorized"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "Unauthorized"),
        ],
    )
    def test_invalid_auth(
        self,
        invalid_auth: RAGFlowWebApiAuth | None,
        expected_code: int,
        expected_message: str,
        web_api_auth: RAGFlowWebApiAuth,
    ) -> None:
        """Test team creation with invalid or missing authentication."""
        # Try to create team with invalid auth
        team_payload: dict[str, str] = {
            "name": "Test Team Auth",
        }
        res: dict[str, Any] = create_team(invalid_auth, team_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestTeamCreate:
    """Comprehensive tests for team creation API."""

    @pytest.mark.p1
    def test_create_team_with_name_and_user_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with name and user_id."""
        # Create team (user_id is optional, defaults to current authenticated user)
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {
            "name": team_name,
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 0, res
        assert "data" in res
        assert res["data"]["name"] == team_name
        assert "owner_id" in res["data"]
        assert "id" in res["data"]
        assert "deleted successfully" not in res["message"].lower()
        assert "created successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_create_team_missing_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team without name."""
        # Try to create team without name
        team_payload: dict[str, str] = {}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 101
        assert "name" in res["message"].lower() or "required" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_create_team_empty_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with empty name."""
        # Try to create team with empty name
        team_payload: dict[str, str] = {"name": ""}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 101
        assert "name" in res["message"].lower() or "required" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_create_team_name_too_long(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with name exceeding 100 characters."""
        # Try to create team with name too long
        long_name: str = "A" * 101
        team_payload: dict[str, str] = {"name": long_name}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 101
        assert "100" in res["message"] or "length" in res["message"].lower()

    @pytest.mark.p1
    def test_create_team_invalid_user_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with non-existent user_id."""
        team_payload: dict[str, str] = {
            "name": "Test Team Invalid User",
            "user_id": "non_existent_user_id_12345",
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 102
        assert "not found" in res["message"].lower()

    @pytest.mark.p1
    def test_create_team_missing_user_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team without user_id (should use current authenticated user)."""
        team_payload: dict[str, str] = {"name": "Test Team No User"}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        # Should succeed since user_id defaults to current authenticated user
        assert res["code"] == 0
        assert "data" in res
        assert "owner_id" in res["data"]
        assert "created successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_create_team_response_structure(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test that team creation returns the expected response structure."""
        # Create team
        team_name: str = f"Test Team Structure {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {
            "name": team_name,
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 0
        assert "data" in res
        assert isinstance(res["data"], dict)
        assert "id" in res["data"]
        assert "name" in res["data"]
        assert "owner_id" in res["data"]
        assert res["data"]["name"] == team_name
        assert "message" in res
        assert "created successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_create_multiple_teams_same_user(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating multiple teams for the same user."""
        # Create first team
        team_name_1: str = f"Team 1 {uuid.uuid4().hex[:8]}"
        team_payload_1: dict[str, str] = {
            "name": team_name_1,
        }
        res1: dict[str, Any] = create_team(web_api_auth, team_payload_1)
        assert res1["code"] == 0, res1
        team_id_1: str = res1["data"]["id"]

        # Create second team
        team_name_2: str = f"Team 2 {uuid.uuid4().hex[:8]}"
        team_payload_2: dict[str, str] = {
            "name": team_name_2,
        }
        res2: dict[str, Any] = create_team(web_api_auth, team_payload_2)
        assert res2["code"] == 0, res2
        team_id_2: str = res2["data"]["id"]

        # Verify teams are different
        assert team_id_1 != team_id_2
        assert res1["data"]["name"] == team_name_1
        assert res2["data"]["name"] == team_name_2

    @pytest.mark.p2
    def test_create_team_with_whitespace_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with whitespace-only name."""
        # Try to create team with whitespace-only name
        team_payload: dict[str, str] = {
            "name": "   ",
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        # Should fail validation
        assert res["code"] == 101
        assert "name" in res["message"].lower() or "required" in res[
            "message"
        ].lower()

    @pytest.mark.p2
    def test_create_team_special_characters_in_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with special characters in name."""
        # Create team with special characters
        team_name: str = f"Team-{uuid.uuid4().hex[:8]}_Test!"
        team_payload: dict[str, str] = {
            "name": team_name,
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        # Should succeed if special chars are allowed
        assert res["code"] in (0, 101)

    @pytest.mark.p2
    def test_create_team_empty_payload(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with empty payload."""
        team_payload: dict[str, Any] = {}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 101
        assert "required" in res["message"].lower() or "name" in res[
            "message"
        ].lower()

    @pytest.mark.p3
    def test_create_team_unicode_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with unicode characters in name."""
        # Create team with unicode name
        team_name: str = f"团队{uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {
            "name": team_name,
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        # Should succeed if unicode is supported
        assert res["code"] in (0, 101)

    @pytest.mark.p2
    def test_team_creation_with_custom_models(
        self, web_api_auth: RAGFlowWebApiAuth
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
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        
        # Should succeed with defaults if custom models not specified
        assert res["code"] == 0, res
        assert res["data"]["name"] == team_name
        assert "id" in res["data"]

    @pytest.mark.p2
    def test_multiple_teams_same_name_allowed(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test that multiple teams can have the same name."""
        team_name: str = f"Duplicate Name {uuid.uuid4().hex[:8]}"
        
        # Create first team
        res1: dict[str, Any] = create_team(web_api_auth, {"name": team_name})
        assert res1["code"] == 0, res1
        team_id_1: str = res1["data"]["id"]
        
        # Create second team with same name
        res2: dict[str, Any] = create_team(web_api_auth, {"name": team_name})
        assert res2["code"] == 0, res2
        team_id_2: str = res2["data"]["id"]
        
        # Teams should have different IDs
        assert team_id_1 != team_id_2, "Teams should have unique IDs"
        assert res1["data"]["name"] == res2["data"]["name"], (
            "Both teams should have the same name"
        )

    @pytest.mark.p2
    def test_team_creation_with_credit_limit(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating team with custom credit limit."""
        team_name: str = f"Credit Test Team {uuid.uuid4().hex[:8]}"
        custom_credit: int = 1000
        
        team_payload: dict[str, Any] = {
            "name": team_name,
            "credit": custom_credit,
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        
        # Should succeed
        assert res["code"] == 0, res
        # Note: Credit may not be in response, but should be set internally

    @pytest.mark.p2
    def test_team_name_with_special_characters(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test team names with special characters."""
        special_names: list[str] = [
            f"Team-{uuid.uuid4().hex[:4]}_Test!",
            f"Team & Co. {uuid.uuid4().hex[:4]}",
            f"Team @{uuid.uuid4().hex[:4]}",
            f"团队{uuid.uuid4().hex[:4]}",  # Unicode
        ]
        
        for name in special_names:
            res: dict[str, Any] = create_team(web_api_auth, {"name": name})
            # Should either accept or reject with clear message
            if res["code"] == 0:
                assert res["data"]["name"] == name, (
                    f"Team name should be preserved: {name}"
                )
            # If rejected, should have clear error
            # (Current implementation accepts special chars)

    @pytest.mark.p2
    def test_team_creation_default_owner(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test that team creator is set as owner by default."""
        team_name: str = f"Owner Test Team {uuid.uuid4().hex[:8]}"
        res: dict[str, Any] = create_team(web_api_auth, {"name": team_name})
        
        assert res["code"] == 0, res
        assert "owner_id" in res["data"], "Owner ID should be in response"
        # Owner should be the authenticated user
        # (Cannot verify without knowing web_api_auth user ID)

    @pytest.mark.p2
    def test_concurrent_team_creation(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test concurrent team creation."""
        import concurrent.futures
        
        def create_test_team(index: int) -> dict[str, Any]:
            team_name: str = f"Concurrent Team {index}_{uuid.uuid4().hex[:8]}"
            return create_team(web_api_auth, {"name": team_name})
        
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

    @pytest.mark.p2
    def test_team_with_invalid_model_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test team creation with invalid model ID."""
        team_name: str = f"Invalid Model Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {
            "name": team_name,
            "llm_id": "invalid_nonexistent_model_id_12345",
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        
        # Should reject with clear error message
        assert res["code"] != 0, "Invalid model ID should be rejected"
        assert (
            "model" in res["message"].lower()
            or "not found" in res["message"].lower()
            or "invalid" in res["message"].lower()
        ), "Error message should mention model issue"

    @pytest.mark.p2
    def test_team_creation_with_negative_credit(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test team creation with negative credit."""
        team_name: str = f"Negative Credit Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, Any] = {
            "name": team_name,
            "credit": -100,
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        
        # Should reject negative credit
        assert res["code"] != 0, "Negative credit should be rejected"
        assert "credit" in res["message"].lower(), (
            "Error message should mention credit"
        )

    @pytest.mark.p2
    def test_team_creation_empty_json_payload(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test team creation with completely empty payload."""
        res: dict[str, Any] = create_team(web_api_auth, {})
        
        # Should reject with clear error
        assert res["code"] != 0, "Empty payload should be rejected"
        assert (
            "name" in res["message"].lower()
            or "required" in res["message"].lower()
        ), "Error should mention missing name"

    @pytest.mark.p3
    def test_team_unicode_name(
        self, web_api_auth: RAGFlowWebApiAuth
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
            res: dict[str, Any] = create_team(web_api_auth, {"name": name})
            
            # Should handle unicode properly
            if res["code"] == 0:
                # Verify unicode is preserved (may be normalized)
                assert len(res["data"]["name"]) > 0, (
                    "Team name should not be empty after unicode"
                )

    @pytest.mark.p3
    def test_team_creation_with_all_optional_params(
        self, web_api_auth: RAGFlowWebApiAuth
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
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        
        # Should succeed
        assert res["code"] == 0, res
        assert res["data"]["name"] == team_name

    @pytest.mark.p3
    def test_team_max_name_length(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test team with maximum allowed name length."""
        # API spec says max 100 characters
        max_name: str = "A" * 100
        res: dict[str, Any] = create_team(web_api_auth, {"name": max_name})
        
        # Should accept 100 characters
        assert res["code"] == 0, "100-character name should be accepted"
        assert res["data"]["name"] == max_name

    @pytest.mark.p3
    def test_team_name_just_over_limit(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test team with name just over limit."""
        # 101 characters (1 over limit)
        long_name: str = "A" * 101
        res: dict[str, Any] = create_team(web_api_auth, {"name": long_name})
        
        # Should reject
        assert res["code"] != 0, "101-character name should be rejected"
        assert (
            "100" in res["message"]
            or "length" in res["message"].lower()
            or "long" in res["message"].lower()
        ), "Error should mention length limit"

    @pytest.mark.p3
    def test_team_creation_idempotency(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test that repeated team creation creates separate teams."""
        team_name: str = f"Idempotency Test {uuid.uuid4().hex[:8]}"
        payload: dict[str, str] = {"name": team_name}
        
        # Create same team twice
        res1: dict[str, Any] = create_team(web_api_auth, payload)
        res2: dict[str, Any] = create_team(web_api_auth, payload)
        
        # Both should succeed and create different teams
        assert res1["code"] == 0, res1
        assert res2["code"] == 0, res2
        assert res1["data"]["id"] != res2["data"]["id"], (
            "Should create different teams, not be idempotent"
        )

    @pytest.mark.p3
    def test_team_with_parser_ids(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test team creation with custom parser IDs."""
        team_name: str = f"Parser Test {uuid.uuid4().hex[:8]}"
        # parser_ids is typically a comma-separated string
        team_payload: dict[str, str] = {
            "name": team_name,
            "parser_ids": "naive,qa,table,paper,book,laws,presentation,manual,wiki",
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        
        # Should accept valid parser IDs
        assert res["code"] == 0, res

