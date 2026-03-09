#!/usr/bin/env python3
"""
Quick test runner for metadata feature unit tests.
Runs only the new metadata-related tests to verify coverage improvements.
"""
import subprocess
import sys

def run_tests():
    """Run metadata feature tests."""
    test_commands = [
        # Run only the new metadata tests in chunk_app
        [
            "pytest", "-v", "-s", "--tb=short",
            "test/testcases/test_web_api/test_chunk_app/test_chunk_routes_unit.py",
            "-k", "metadata"
        ],
    ]
    
    print("=" * 60)
    print("Running Metadata Feature Unit Tests")
    print("=" * 60)
    print()
    
    all_passed = True
    for cmd in test_commands:
        print(f"Running: {' '.join(cmd)}")
        print("-" * 60)
        result = subprocess.run(cmd, capture_output=False)
        print()
        
        if result.returncode != 0:
            all_passed = False
            print(f"FAILED with exit code {result.returncode}")
        else:
            print("PASSED")
        print()
    
    print("=" * 60)
    if all_passed:
        print("✓ All metadata tests PASSED!")
        print("=" * 60)
        return 0
    else:
        print("✗ Some tests FAILED")
        print("=" * 60)
        return 1

if __name__ == "__main__":
    sys.exit(run_tests())
