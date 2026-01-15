#!/usr/bin/env python3
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

import sys
import os
import argparse
import subprocess
from pathlib import Path
from typing import List


class Colors:
    """ANSI color codes for terminal output"""
    RED = '\033[0;31m'
    GREEN = '\033[0;32m'
    YELLOW = '\033[1;33m'
    BLUE = '\033[0;34m'
    NC = '\033[0m'  # No Color


class TestRunner:
    """RAGFlow Test Runner - Supports unit, integration, contract, and benchmark tests"""

    def __init__(self):
        self.project_root = Path(__file__).parent.resolve()
        self.test_root = Path(self.project_root / 'test')
        
        # Test directories
        self.unit_dir = self.test_root / 'unit_test'
        self.integration_dir = self.test_root / 'integration'
        self.contract_dir = self.test_root / 'api_contract'
        self.benchmark_dir = self.test_root / 'benchmark'
        
        # Default options
        self.coverage = False
        self.parallel = False
        self.verbose = False
        self.markers = ""
        self.test_type = "all"  # unit, integration, contract, benchmark, or all

        # Python interpreter path
        self.python = sys.executable

    @staticmethod
    def print_info(message: str) -> None:
        """Print informational message"""
        print(f"{Colors.BLUE}[INFO]{Colors.NC} {message}")

    @staticmethod
    def print_error(message: str) -> None:
        """Print error message"""
        print(f"{Colors.RED}[ERROR]{Colors.NC} {message}")

    @staticmethod
    def print_warning(message: str) -> None:
        """Print warning message"""
        print(f"{Colors.YELLOW}[WARNING]{Colors.NC} {message}")

    def check_environment(self) -> None:
        """Check for required environment variables based on test type"""
        if self.test_type in ["integration", "contract", "all"]:
            # Check for integration/contract test requirements
            missing = []
            
            if not os.getenv("ZHIPU_AI_API_KEY"):
                missing.append("ZHIPU_AI_API_KEY")
            
            if missing:
                self.print_warning("Missing environment variables for integration/contract tests:")
                for var in missing:
                    print(f"    - {var}")
                self.print_info("Integration and contract tests may be skipped; the runner will continue.")
                print()

    @staticmethod
    def show_usage() -> None:
        """Display usage information"""
        usage = """
RAGFlow Test Runner
Usage: python run_tests.py [OPTIONS]

TEST TYPES:
    --unit                  Run unit tests only (test/unit_test/)
    --integration           Run integration tests only (test/integration/)
    --contract              Run API contract tests only (test/api_contract/)
    --benchmark             Run benchmark tests only (test/benchmark/)
    --all                   Run all tests (default)

OPTIONS:
    -h, --help              Show this help message
    -c, --coverage          Run tests with coverage report
    -p, --parallel          Run tests in parallel (requires pytest-xdist)
    -v, --verbose           Verbose output
    -t, --test FILE         Run specific test file or directory
    -m, --markers MARKERS   Run tests with specific markers

EXAMPLES:
    # Run all tests
    python run_tests.py

    # Run only unit tests with coverage
    python run_tests.py --unit --coverage

    # Run integration tests in parallel
    python run_tests.py --integration --parallel

    # Run contract tests with verbose output
    python run_tests.py --contract --verbose

    # Run specific test file
    python run_tests.py --test test/unit_test/services/test_dialog_service.py

    # Run tests with specific markers
    python run_tests.py --markers "slow"

"""
        print(usage)

    def get_test_paths(self) -> List[Path]:
        """Get test paths based on test type"""
        paths = []
        
        if self.test_type == "unit":
            paths.append(self.unit_dir)
        elif self.test_type == "integration":
            paths.append(self.integration_dir)
        elif self.test_type == "contract":
            paths.append(self.contract_dir)
        elif self.test_type == "benchmark":
            paths.append(self.benchmark_dir)
        else:  # all
            paths.extend([self.unit_dir, self.integration_dir, self.contract_dir, self.benchmark_dir])
        
        # Filter out non-existent paths
        return [p for p in paths if p.exists()]

    def build_pytest_command(self) -> List[str]:
        """Build the pytest command arguments"""
        test_paths = self.get_test_paths()
        
        if not test_paths:
            raise ValueError(f"No test directories found for test type: {self.test_type}")
        
        # Use python -m pytest to ensure virtual environment is used
        cmd = [self.python, "-m", "pytest"] + [str(p) for p in test_paths]

        # Add markers
        if self.markers:
            cmd.extend(["-m", self.markers])

        # Add verbose flag
        if self.verbose:
            cmd.extend(["-vv"])
        else:
            cmd.append("-v")

        # Add coverage
        if self.coverage:
            # Coverage for main source directories
            source_paths = ["api", "rag", "agent", "common", "deepdoc", "graphrag"]
            for source in source_paths:
                source_dir = self.project_root / source
                if source_dir.exists():
                    cmd.extend(["--cov", str(source_dir)])
            
            cmd.extend([
                "--cov-report", "html",
                "--cov-report", "term",
                "--cov-report", "term-missing"
            ])

        # Add parallel execution
        if self.parallel:
            # Try to get number of CPU cores
            try:
                import multiprocessing
                cpu_count = multiprocessing.cpu_count()
                cmd.extend(["-n", str(cpu_count)])
            except ImportError:
                # Fallback to auto if multiprocessing not available
                cmd.extend(["-n", "auto"])

        # Add default options from pyproject.toml if it exists
        pyproject_path = self.project_root / "pyproject.toml"
        if pyproject_path.exists():
            cmd.extend(["--config-file", str(pyproject_path)])

        return cmd

    def run_tests(self) -> bool:
        """Execute the pytest command"""
        # Change to test directory
        os.chdir(self.project_root)

        # Check environment variables
        self.check_environment()

        # Build command
        cmd = self.build_pytest_command()

        # Print test configuration
        test_paths = self.get_test_paths()
        self.print_info("Running RAGFlow Tests")
        self.print_info("=" * 40)
        self.print_info(f"Test Type: {self.test_type}")
        self.print_info("Test Directories:")
        for path in test_paths:
            self.print_info(f"  - {path.relative_to(self.project_root)}")
        self.print_info(f"Coverage: {self.coverage}")
        self.print_info(f"Parallel: {self.parallel}")
        self.print_info(f"Verbose: {self.verbose}")

        if self.markers:
            self.print_info(f"Markers: {self.markers}")

        print(f"\n{Colors.BLUE}[EXECUTING]{Colors.NC} {' '.join(cmd)}\n")

        # Run pytest
        try:
            result = subprocess.run(cmd, check=False)

            if result.returncode == 0:
                print(f"\n{Colors.GREEN}[SUCCESS]{Colors.NC} All tests passed!")

                if self.coverage:
                    coverage_dir = self.project_root / "htmlcov"
                    if coverage_dir.exists():
                        index_file = coverage_dir / "index.html"
                        print(f"\n{Colors.BLUE}[INFO]{Colors.NC} Coverage report generated:")
                        print(f"    {index_file}")
                        print("\nOpen with:")
                        print(f"    - Windows: start {index_file}")
                        print(f"    - macOS: open {index_file}")
                        print(f"    - Linux: xdg-open {index_file}")

                return True
            else:
                print(f"\n{Colors.RED}[FAILURE]{Colors.NC} Some tests failed!")
                return False

        except KeyboardInterrupt:
            print(f"\n{Colors.YELLOW}[INTERRUPTED]{Colors.NC} Test execution interrupted by user")
            return False
        except Exception as e:
            self.print_error(f"Failed to execute tests: {e}")
            return False

    def parse_arguments(self) -> bool:
        """Parse command line arguments"""
        parser = argparse.ArgumentParser(
            description="RAGFlow Test Runner",
            formatter_class=argparse.RawDescriptionHelpFormatter,
            epilog="""
Examples:
  python run_tests.py                    # Run all tests
  python run_tests.py --unit             # Run unit tests only
  python run_tests.py --integration      # Run integration tests only
  python run_tests.py --contract         # Run API contract tests only
  python run_tests.py --unit --coverage  # Run unit tests with coverage
  python run_tests.py --parallel         # Run in parallel
  python run_tests.py --test test/unit_test/services/test_dialog_service.py  # Run specific test
"""
        )

        # Test type selection (mutually exclusive)
        test_type_group = parser.add_mutually_exclusive_group()
        test_type_group.add_argument(
            "--unit",
            action="store_true",
            help="Run unit tests only (test/unit_test/)"
        )
        test_type_group.add_argument(
            "--integration",
            action="store_true",
            help="Run integration tests only (test/integration/)"
        )
        test_type_group.add_argument(
            "--contract",
            action="store_true",
            help="Run API contract tests only (test/api_contract/)"
        )
        test_type_group.add_argument(
            "--benchmark",
            action="store_true",
            help="Run benchmark tests only (test/benchmark/)"
        )
        test_type_group.add_argument(
            "--all",
            action="store_true",
            help="Run all tests (default)"
        )

        parser.add_argument(
            "-c", "--coverage",
            action="store_true",
            help="Run tests with coverage report"
        )

        parser.add_argument(
            "-p", "--parallel",
            action="store_true",
            help="Run tests in parallel (requires pytest-xdist)"
        )

        parser.add_argument(
            "-v", "--verbose",
            action="store_true",
            help="Verbose output"
        )

        parser.add_argument(
            "-t", "--test",
            type=str,
            default="",
            help="Run specific test file or directory"
        )

        parser.add_argument(
            "-m", "--markers",
            type=str,
            default="",
            help="Run tests with specific markers"
        )

        try:
            args = parser.parse_args()

            # Set test type
            if args.unit:
                self.test_type = "unit"
            elif args.integration:
                self.test_type = "integration"
            elif args.contract:
                self.test_type = "contract"
            elif args.benchmark:
                self.test_type = "benchmark"
            else:
                self.test_type = "all"

            # Set options
            self.coverage = args.coverage
            self.parallel = args.parallel
            self.verbose = args.verbose
            self.markers = args.markers

            return True

        except SystemExit:
            # argparse already printed help, just exit
            return False
        except Exception as e:
            self.print_error(f"Error parsing arguments: {e}")
            return False

    def run(self) -> int:
        """Main execution method"""
        # Parse command line arguments
        if not self.parse_arguments():
            return 1

        # Run tests
        success = self.run_tests()

        return 0 if success else 1


def main():
    """Entry point"""
    runner = TestRunner()
    return runner.run()


if __name__ == "__main__":
    sys.exit(main())