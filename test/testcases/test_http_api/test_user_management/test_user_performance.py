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
"""Performance and load tests for user management APIs."""

from __future__ import annotations

import time
import uuid
from concurrent.futures import Future, ThreadPoolExecutor, as_completed
from typing import Any

import pytest

from ..common import create_user, list_users
from libs.auth import RAGFlowWebApiAuth


@pytest.mark.performance
@pytest.mark.usefixtures("clear_users")
class TestUserPerformance:
    """Performance and load tests for user management."""

    @pytest.mark.p2
    def test_list_users_performance_small_dataset(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test list_users performance with small dataset."""
        # Create 20 users
        created_users: list[str] = []
        for i in range(20):
            unique_email: str = f"perf_small_{i}_{uuid.uuid4().hex[:4]}@example.com"
            res: dict[str, Any] = create_user(
                web_api_auth,
                {
                    "nickname": f"user_{i}",
                    "email": unique_email,
                    "password": "test123",
                },
            )
            if res["code"] == 0:
                created_users.append(res["data"]["id"])
        
        # Test list performance without pagination
        start: float = time.time()
        res: dict[str, Any] = list_users(web_api_auth)
        duration: float = time.time() - start
        
        assert res["code"] == 0, res
        assert duration < 2.0, (
            f"List operation took {duration}s, should be under 2s"
        )

    @pytest.mark.p2
    def test_list_users_pagination_performance(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test pagination performance with moderate dataset."""
        # Create 50 users
        for i in range(50):
            unique_email: str = f"perf_test_{i}_{uuid.uuid4().hex[:4]}@example.com"
            create_user(
                web_api_auth,
                {
                    "nickname": f"user_{i}",
                    "email": unique_email,
                    "password": "test123",
                },
            )
        
        # Test pagination performance
        start: float = time.time()
        res: dict[str, Any] = list_users(
            web_api_auth, params={"page": 1, "page_size": 10}
        )
        duration: float = time.time() - start
        
        assert res["code"] == 0, res
        assert len(res["data"]) <= 10, "Should return requested page size"
        assert duration < 1.0, (
            f"Paginated list took {duration}s, should be under 1s"
        )

    @pytest.mark.p3
    def test_concurrent_user_creation(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test concurrent user creation without conflicts."""
        count: int = 20
        
        def create_test_user(index: int) -> dict[str, Any]:
            unique_email: str = f"concurrent_{index}_{uuid.uuid4().hex[:8]}@example.com"
            return create_user(
                web_api_auth,
                {
                    "nickname": f"user_{index}",
                    "email": unique_email,
                    "password": "test123",
                },
            )
        
        # Create 20 users concurrently with 5 workers
        start: float = time.time()
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures: list[Future[dict[str, Any]]] = [
                executor.submit(create_test_user, i) for i in range(count)
            ]
            results: list[dict[str, Any]] = [
                f.result() for f in as_completed(futures)
            ]
        duration: float = time.time() - start
        
        # All should succeed
        success_count: int = sum(1 for r in results if r["code"] == 0)
        assert success_count == count, (
            f"Expected {count} successful creations, got {success_count}"
        )
        
        # Should complete reasonably quickly
        # 20 users with 5 workers ~= 4 batches, allow 10 seconds
        assert duration < 10.0, (
            f"Concurrent creation took {duration}s, should be under 10s"
        )

    @pytest.mark.p3
    def test_user_creation_response_time(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test individual user creation response time."""
        response_times: list[float] = []
        
        for i in range(10):
            unique_email: str = f"timing_{i}_{uuid.uuid4().hex[:8]}@example.com"
            start: float = time.time()
            res: dict[str, Any] = create_user(
                web_api_auth,
                {
                    "nickname": f"user_{i}",
                    "email": unique_email,
                    "password": "test123",
                },
            )
            duration: float = time.time() - start
            
            assert res["code"] == 0, f"User creation failed: {res}"
            response_times.append(duration)
        
        # Calculate statistics
        avg_time: float = sum(response_times) / len(response_times)
        max_time: float = max(response_times)
        
        # Average response time should be reasonable
        assert avg_time < 1.0, (
            f"Average user creation time {avg_time}s should be under 1s"
        )
        # Max response time shouldn't spike too high
        assert max_time < 3.0, (
            f"Max user creation time {max_time}s should be under 3s"
        )

    @pytest.mark.p3
    def test_sequential_vs_concurrent_creation_comparison(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Compare sequential vs concurrent user creation performance."""
        count: int = 10
        
        # Sequential creation
        sequential_start: float = time.time()
        for i in range(count):
            unique_email: str = f"seq_{i}_{uuid.uuid4().hex[:8]}@example.com"
            create_user(
                web_api_auth,
                {
                    "nickname": f"seq_user_{i}",
                    "email": unique_email,
                    "password": "test123",
                },
            )
        sequential_duration: float = time.time() - sequential_start
        
        # Concurrent creation
        def create_concurrent_user(index: int) -> dict[str, Any]:
            unique_email: str = f"conc_{index}_{uuid.uuid4().hex[:8]}@example.com"
            return create_user(
                web_api_auth,
                {
                    "nickname": f"conc_user_{index}",
                    "email": unique_email,
                    "password": "test123",
                },
            )
        
        concurrent_start: float = time.time()
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures: list[Future[dict[str, Any]]] = [
                executor.submit(create_concurrent_user, i) for i in range(count)
            ]
            concurrent_results: list[dict[str, Any]] = [
                f.result() for f in as_completed(futures)
            ]
        concurrent_duration: float = time.time() - concurrent_start
        
        # Concurrent should be faster (or at least not significantly slower)
        # Allow some overhead for thread management
        speedup_ratio: float = sequential_duration / concurrent_duration
        
        # Log performance metrics for analysis
        print(f"\nPerformance Comparison ({count} users):")
        print(f"Sequential: {sequential_duration:.2f}s")
        print(f"Concurrent: {concurrent_duration:.2f}s")
        print(f"Speedup: {speedup_ratio:.2f}x")
        
        # Concurrent should provide some benefit (at least not be slower)
        # With 5 workers, expect at least some improvement
        assert concurrent_duration <= sequential_duration * 1.2, (
            "Concurrent creation should not be significantly slower than sequential"
        )

    @pytest.mark.p3
    def test_pagination_consistency_under_load(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test pagination consistency during concurrent modifications."""
        # Create initial set of users
        initial_count: int = 30
        for i in range(initial_count):
            unique_email: str = f"pag_{i}_{uuid.uuid4().hex[:8]}@example.com"
            create_user(
                web_api_auth,
                {
                    "nickname": f"user_{i}",
                    "email": unique_email,
                    "password": "test123",
                },
            )
        
        # Test pagination while users are being created
        def paginate_users() -> dict[str, Any]:
            return list_users(web_api_auth, params={"page": 1, "page_size": 10})
        
        def create_more_users() -> None:
            for i in range(5):
                unique_email: str = f"new_{i}_{uuid.uuid4().hex[:8]}@example.com"
                create_user(
                    web_api_auth,
                    {
                        "nickname": f"new_user_{i}",
                        "email": unique_email,
                        "password": "test123",
                    },
                )
        
        with ThreadPoolExecutor(max_workers=3) as executor:
            # Start pagination requests
            pag_futures: list[Future] = [
                executor.submit(paginate_users) for _ in range(5)
            ]
            # Start creation requests
            create_future: Future = executor.submit(create_more_users)
            
            # Wait for all to complete
            pag_results: list[dict[str, Any]] = [
                f.result() for f in pag_futures
            ]
            create_future.result()
        
        # All pagination requests should succeed
        assert all(r["code"] == 0 for r in pag_results), (
            "Pagination should remain stable during concurrent modifications"
        )

    @pytest.mark.p3
    def test_memory_efficiency_large_list(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test memory efficiency when listing many users."""
        # Create 100 users
        for i in range(100):
            unique_email: str = f"mem_{i}_{uuid.uuid4().hex[:8]}@example.com"
            create_user(
                web_api_auth,
                {
                    "nickname": f"user_{i}",
                    "email": unique_email,
                    "password": "test123",
                },
            )
        
        # List all users (without pagination)
        res: dict[str, Any] = list_users(web_api_auth)
        
        assert res["code"] == 0, res
        # Should return results without memory issues
        assert isinstance(res["data"], list), "Should return list"
        # Response should not be excessively large
        # (This is a basic check; real memory profiling would need additional tools)

    @pytest.mark.p3
    @pytest.mark.skip(reason="Stress test - run manually")
    def test_sustained_load(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test system stability under sustained load (manual run)."""
        duration_seconds: int = 60  # Run for 1 minute
        requests_per_second: int = 5
        
        start_time: float = time.time()
        request_count: int = 0
        error_count: int = 0
        
        while time.time() - start_time < duration_seconds:
            batch_start: float = time.time()
            
            # Send requests_per_second requests
            for i in range(requests_per_second):
                unique_email: str = f"load_{request_count}_{uuid.uuid4().hex[:8]}@example.com"
                res: dict[str, Any] = create_user(
                    web_api_auth,
                    {
                        "nickname": f"user_{request_count}",
                        "email": unique_email,
                        "password": "test123",
                    },
                )
                request_count += 1
                if res["code"] != 0:
                    error_count += 1
            
            # Wait to maintain requests_per_second rate
            elapsed: float = time.time() - batch_start
            sleep_time: float = 1.0 - elapsed
            if sleep_time > 0:
                time.sleep(sleep_time)
        
        total_duration: float = time.time() - start_time
        actual_rps: float = request_count / total_duration
        error_rate: float = error_count / request_count if request_count > 0 else 0
        
        print(f"\nSustained Load Test Results:")
        print(f"Duration: {total_duration:.2f}s")
        print(f"Total Requests: {request_count}")
        print(f"Actual RPS: {actual_rps:.2f}")
        print(f"Error Rate: {error_rate:.2%}")
        
        # Error rate should be low
        assert error_rate < 0.05, (
            f"Error rate {error_rate:.2%} should be under 5%"
        )

    @pytest.mark.p3
    def test_large_payload_handling(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test handling of large request payloads."""
        # Create user with large nickname (but within limits)
        large_nickname: str = "A" * 200  # 200 characters
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        
        start: float = time.time()
        res: dict[str, Any] = create_user(
            web_api_auth,
            {
                "nickname": large_nickname,
                "email": unique_email,
                "password": "test123" * 10,  # Longer password
            },
        )
        duration: float = time.time() - start
        
        # Should handle large payloads efficiently
        assert duration < 2.0, (
            f"Large payload took {duration}s, should be under 2s"
        )
        
        if res["code"] == 0:
            # Verify data was stored correctly
            assert len(res["data"]["nickname"]) <= 255, (
                "Nickname should be capped at reasonable length"
            )
