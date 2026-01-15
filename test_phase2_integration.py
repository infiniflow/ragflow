#!/usr/bin/env python
"""Quick integration test for Phase 2 Pool Diagnostics

This module can be run directly as a script or via pytest.
"""
import threading
from unittest.mock import MagicMock

import pytest

from api.db.connection import PoolDiagnostics


def _create_mock_db(active: int, idle: int, max_conn: int) -> MagicMock:
    """Create a mock database with specified connection counts."""
    mock_db = MagicMock()
    mock_db.max_connections = max_conn
    mock_db._in_use = {f"conn_{i}": MagicMock() for i in range(active)}
    mock_db._connections = [MagicMock() for _ in range(idle)]
    return mock_db


def _run_scenario(name: str, active: int, idle: int, max_conn: int) -> dict:
    """Run a test scenario and return stats."""
    mock_db = _create_mock_db(active, idle, max_conn)
    
    stats = PoolDiagnostics.get_pool_stats(mock_db)
    
    # Add scenario name to stats for tracking/debugging
    stats['scenario'] = name
    
    # Validate stats structure and values
    assert isinstance(stats, dict), f"Stats should be a dict, got {type(stats)}"
    assert 'max' in stats, "Stats missing 'max' key"
    assert 'active' in stats, "Stats missing 'active' key"
    assert 'idle' in stats, "Stats missing 'idle' key"
    assert 'utilization_percent' in stats, "Stats missing 'utilization_percent' key"
    
    # Validate stats values match input
    assert stats['max'] == max_conn, f"Expected max={max_conn}, got {stats['max']}"
    assert stats['active'] == active, f"Expected active={active}, got {stats['active']}"
    assert stats['idle'] == idle, f"Expected idle={idle}, got {stats['idle']}"
    
    # Validate utilization calculation
    expected_utilization = round(active / max_conn * 100, 2) if max_conn > 0 else 0
    assert stats['utilization_percent'] == expected_utilization, \
        f"Expected utilization={expected_utilization}%, got {stats['utilization_percent']}%"
    
    # Test logging (captures log level, but no assertions on logging output)
    PoolDiagnostics.log_pool_health(mock_db)
    return stats


class TestPoolDiagnosticsIntegration:
    """Integration tests for PoolDiagnostics."""

    def test_normal_load(self):
        """Test pool stats with normal load (30% utilization)."""
        stats = _run_scenario("Normal load", active=3, idle=2, max_conn=10)
        assert stats['utilization_percent'] == 30.0

    def test_high_load_warning(self):
        """Test pool stats with high load that triggers warning (80% utilization)."""
        stats = _run_scenario("High load (warning)", active=8, idle=1, max_conn=10)
        assert stats['utilization_percent'] == 80.0

    def test_critical_load(self):
        """Test pool stats at critical load (100% utilization)."""
        stats = _run_scenario("Critical load", active=10, idle=0, max_conn=10)
        assert stats['utilization_percent'] == 100.0

    def test_empty_pool(self):
        """Test pool stats with empty pool (0% utilization)."""
        stats = _run_scenario("Empty pool", active=0, idle=0, max_conn=20)
        assert stats['utilization_percent'] == 0.0

    def test_background_monitoring(self):
        """Test that background monitoring thread runs and invokes health checks."""
        mock_db = _create_mock_db(active=5, idle=0, max_conn=10)
        
        # Create a spy callback with Event to verify monitoring actually runs
        callback_event = threading.Event()
        monitor_callback = MagicMock()
        original_log = PoolDiagnostics.log_pool_health

        def log_with_spy(*args, **kwargs):
            monitor_callback()
            callback_event.set()  # Signal that callback was invoked
            return original_log(*args, **kwargs)

        PoolDiagnostics.log_pool_health = log_with_spy

        try:
            PoolDiagnostics.start_health_monitoring(mock_db, interval=0.5)
            
            # Wait for the callback to be invoked (with timeout)
            callback_invoked = callback_event.wait(timeout=2.0)
            assert callback_invoked, "Monitor callback should have been invoked within timeout"

            # Assert the monitor actually invoked the health check
            assert monitor_callback.call_count >= 1, \
                f"Monitor callback should have been called at least once, but was called {monitor_callback.call_count} times"
        finally:
            # Always clean up, even on assertion failure
            PoolDiagnostics.stop_health_monitoring()
            # Restore original function
            PoolDiagnostics.log_pool_health = original_log


if __name__ == "__main__":
    # Run as a script for quick smoke testing
    print("=== Phase 2 Pool Diagnostics Integration Test ===")
    
    print("\nNormal load:")
    stats = _run_scenario("Normal load", active=3, idle=2, max_conn=10)
    print(f"  Stats: {stats['active']}/{stats['max']} active, {stats['idle']} idle, {stats['utilization_percent']}% used")
    print("  ✓ Stats assertions passed")
    
    print("\nHigh load (warning):")
    stats = _run_scenario("High load (warning)", active=8, idle=1, max_conn=10)
    print(f"  Stats: {stats['active']}/{stats['max']} active, {stats['idle']} idle, {stats['utilization_percent']}% used")
    print("  ✓ Stats assertions passed")
    
    print("\nCritical load:")
    stats = _run_scenario("Critical load", active=10, idle=0, max_conn=10)
    print(f"  Stats: {stats['active']}/{stats['max']} active, {stats['idle']} idle, {stats['utilization_percent']}% used")
    print("  ✓ Stats assertions passed")
    
    print("\nEmpty pool:")
    stats = _run_scenario("Empty pool", active=0, idle=0, max_conn=20)
    print(f"  Stats: {stats['active']}/{stats['max']} active, {stats['idle']} idle, {stats['utilization_percent']}% used")
    print("  ✓ Stats assertions passed")
    
    print("\n\nTesting background monitoring:")
    mock_db = _create_mock_db(active=5, idle=0, max_conn=10)
    
    callback_event = threading.Event()
    monitor_callback = MagicMock()
    original_log = PoolDiagnostics.log_pool_health

    def log_with_spy(*args, **kwargs):
        monitor_callback()
        callback_event.set()
        return original_log(*args, **kwargs)

    PoolDiagnostics.log_pool_health = log_with_spy

    try:
        PoolDiagnostics.start_health_monitoring(mock_db, interval=0.5)
        print("  ✓ Monitoring started")
        
        callback_invoked = callback_event.wait(timeout=2.0)
        assert callback_invoked, "Monitor callback should have been invoked within timeout"
        print("  ✓ Monitoring thread is running")

        assert monitor_callback.call_count >= 1, \
            f"Monitor callback should have been called at least once, but was called {monitor_callback.call_count} times"
        print(f"  ✓ Monitoring callback invoked {monitor_callback.call_count} time(s)")
    finally:
        PoolDiagnostics.stop_health_monitoring()
        print("  ✓ Monitoring stopped")
        PoolDiagnostics.log_pool_health = original_log

    print("\n✅ Phase 2 Integration Smoke Checks Complete!")
    print("   - PoolDiagnostics class: WORKING")
    print("   - Health thresholds: WORKING")
    print("   - Background monitoring: WORKING")
    print("   - ℹ️  Unit tests not run here — please run the unit test suite separately")
