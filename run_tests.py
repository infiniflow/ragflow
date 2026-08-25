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
import platform
from enum import Enum
class Colors(Enum):
    """ANSI color codes for terminal output"""

    RED = "\033[0;31m"
    GREEN = "\033[0;32m"
    YELLOW = "\033[1;33m"
    BLUE = "\033[0;34m"
    BLACK = '\033[30m'
    MAGENTA = '\033[35m'
    CYAN = '\033[36m'
    WHITE = '\033[37m'
    NC = "\033[0m"  # No Color

def _is_color_supported() -> bool:
    """
    Detect whether the current environment supports color output

    Args:
        None
    Returns:
        result(bool): Whether color output is supported
    """
     # Non-interactive terminals do not support color output
    if not sys.stdout.isatty():
        return False
    
    # Handle Windows systems
    if sys.platform.startswith("win"):
        try:
            # Get Windows version number
            win_version = platform.version()
            major, _, build = map(int, win_version.split("."))
            if not (major >= 10 and build >= 10586):
                return False
            from ctypes import windll
            # Actively enable ANSI support for Windows terminal
            INVALID_HANDLE_VALUE = -1
            kernel32 = windll.kernel32
            handle = kernel32.GetStdHandle(-11)  # STD_OUTPUT_HANDLE       
            if handle == INVALID_HANDLE_VALUE:
                return False
            success = kernel32.SetConsoleMode(handle, 7)
            return bool(success)
        except BaseException as e:
            if isinstance(e, (SystemExit, KeyboardInterrupt)):
                raise e
            return False
    # Handle Linux/macOS systems
    else:
        try:
            # Detect color support
            result = subprocess.check_output(
                ["tput", "colors"], 
                stderr=subprocess.DEVNULL,
                text=True
            )
            color_count = int(result.strip())
            return color_count >= 8
        # Explicitly catch tput-related exceptions
        except (subprocess.CalledProcessError, FileNotFoundError, ValueError):
            return False 
        
COLOR_SUPPORT = _is_color_supported()

def set_color(s: str,
              color: str) -> str:
    """
    Wrap input string with specified ANSI terminal color escape sequence.
    Color name argument is case-insensitive. If color is unsupported or invalid,
    returns original raw string without any escape codes.

    Args:
        s: Original text string to be colored
        color: Color enum name, case-insensitive (e.g. "red", "GREEN", "Yellow")
    Returns:
        str: Text wrapped with ANSI color codes if color output is available,
             otherwise the unmodified input string
    Examples:
        >>> set_color(s="hello world",color="red")

    """
    
    if COLOR_SUPPORT:
        return f"{getattr(Colors, color.strip().upper()).value}{s}{Colors.NC.value}"
    return f"{s}"  # pragma: no cover

class TestRunner:
    """RAGFlow Unit Test Runner"""

    def __init__(self):
        self.project_root = Path(__file__).parent.resolve()
        self.ut_dir = Path(self.project_root / "test" / "unit_test")
        # Default options
        self.coverage = False
        self.parallel = False
        self.verbose = False
        self.ignore_syntax_warning = False
        self.markers = ""
        self.test_path = ""
        self.keyword = ""

        # Python interpreter path
        self.python = sys.executable

    @staticmethod
    def print_info(message: str) -> None:
        """Print informational message"""
        print(f"{set_color(s="[INFO]",color="blue")} {message}")

    @staticmethod
    def print_error(message: str) -> None:
        """Print error message"""
        print(f"{set_color(s="[ERROR]",color="red")} {message}")

    @staticmethod
    def show_usage() -> None:
        """Display usage information"""
        usage = """
RAGFlow Unit Test Runner
Usage: python run_tests.py [OPTIONS]

OPTIONS:
    -h, --help              Show this help message
    -c, --coverage          Run tests with coverage report
    -p, --parallel          Run tests in parallel (requires pytest-xdist)
    -i, --ignore            Run tests with "-W ignore::SyntaxWarning" option
    -v, --verbose           Verbose output
    -t, --test FILE         Run specific test file or directory
    -m, --markers MARKERS   Run tests with specific markers (e.g., "unit", "integration")

EXAMPLES:
    # Run all tests
    python run_tests.py

    # Run with coverage
    python run_tests.py --coverage

    # Run in parallel
    python run_tests.py --parallel

    # Run tests with "-W ignore::SyntaxWarning" option
    python run_tests.py --ignore

    # Run specific test file
    python run_tests.py --test services/test_dialog_service.py

    # Run only unit tests
    python run_tests.py --markers "unit"

    # Run tests with coverage and parallel execution
    python run_tests.py --coverage --parallel

"""
        print(usage)

    def build_pytest_command(self) -> List[str]:
        """Build the pytest command arguments"""
        cmd = ["pytest"]
        if self.test_path:
            test_target = Path(self.test_path)
            if not test_target.is_absolute():
                test_target = self.project_root / test_target
            cmd.append(str(test_target))
        else:
            cmd.append(str(self.ut_dir))

        # Add markers
        if self.markers:
            cmd.extend(["-m", self.markers])
        if self.keyword:
            cmd.extend(["-k", self.keyword])

        # Add verbose flag
        if self.verbose:
            cmd.extend(["-vv"])
        else:
            cmd.append("-v")

        # Add coverage
        if self.coverage:
            # Relative path from test directory to source code
            source_path = str(self.project_root / "common")
            cmd.extend(["--cov", source_path, "--cov-report", "html", "--cov-report", "term"])

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

        # Add ignore syntax warning
        if self.ignore_syntax_warning:
            cmd.extend(["-W", "ignore::SyntaxWarning"])

        # Add default options from pyproject.toml if it exists
        pyproject_path = self.project_root / "pyproject.toml"
        if pyproject_path.exists():
            cmd.extend(["--config-file", str(pyproject_path)])

        return cmd

    def run_tests(self) -> bool:
        """Execute the pytest command"""
        # Change to test directory
        os.chdir(self.project_root)

        # Build command
        cmd = self.build_pytest_command()

        # Print test configuration
        self.print_info("Running RAGFlow Unit Tests")
        self.print_info("=" * 40)
        self.print_info(f"Test Directory: {self.ut_dir}")
        self.print_info(f"Coverage: {self.coverage}")
        self.print_info(f"Parallel: {self.parallel}")
        self.print_info(f"Verbose: {self.verbose}")
        if self.test_path:
            self.print_info(f"Test target: {self.test_path}")

        if self.markers:
            self.print_info(f"Markers: {self.markers}")
        if self.keyword:
            self.print_info(f"Keyword: {self.keyword}")

        print(f"\n{set_color(s="[EXECUTING]",color='blue')} {' '.join(cmd)}\n")

        # Run pytest
        try:
            result = subprocess.run(cmd, check=False)

            if result.returncode == 0:
                print(f"\n{set_color(s="[SUCCESS]",color="green")} All tests passed!")

                if self.coverage:
                    coverage_dir = self.ut_dir / "htmlcov"
                    if coverage_dir.exists():
                        index_file = coverage_dir / "index.html"
                        print(f"\n{set_color(s="[INFO]",color="blue")} Coverage report generated:")
                        print(f"    {index_file}")
                        print("\nOpen with:")
                        print(f"    - Windows: start {index_file}")
                        print(f"    - macOS: open {index_file}")
                        print(f"    - Linux: xdg-open {index_file}")

                return True
            else:
                print(f"\n{set_color(s="[FAILURE]",color="red")} Some tests failed!")
                return False

        except KeyboardInterrupt:
            print(f"\n{set_color(s="[INTERRUPTED]",color="yellow")} Test execution interrupted by user")
            return False
        except Exception as e:
            self.print_error(f"Failed to execute tests: {e}")
            return False

    def parse_arguments(self) -> bool:
        """Parse command line arguments"""
        parser = argparse.ArgumentParser(
            description="RAGFlow Unit Test Runner",
            formatter_class=argparse.RawDescriptionHelpFormatter,
            epilog="""
Examples:
  python run_tests.py                    # Run all tests
  python run_tests.py --coverage         # Run with coverage
  python run_tests.py --parallel         # Run in parallel
  python run_tests.py --test services/test_dialog_service.py  # Run specific test
  python run_tests.py --markers "unit"   # Run only unit tests
  python run_tests.py --ignore           # Run with "-W ignore::SyntaxWarning" option
""",
        )

        parser.add_argument("-c", "--coverage", action="store_true", help="Run tests with coverage report")

        parser.add_argument("-p", "--parallel", action="store_true", help="Run tests in parallel (requires pytest-xdist)")

        parser.add_argument("-i", "--ignore", action="store_true", help="Run tests with '-W ignore::SyntaxWarning' ")

        parser.add_argument("-v", "--verbose", action="store_true", help="Verbose output")

        parser.add_argument("-t", "--test", type=str, default="", help="Run specific test file or directory")

        parser.add_argument("-k", "--keyword", type=str, default="", help="Run tests matching keyword expression (pytest -k)")

        parser.add_argument("-m", "--markers", type=str, default="", help="Run tests with specific markers (e.g., 'unit', 'integration')")

        try:
            args = parser.parse_args()

            # Set options
            self.coverage = args.coverage
            self.parallel = args.parallel
            self.verbose = args.verbose
            self.markers = args.markers
            self.ignore_syntax_warning = args.ignore
            self.test_path = args.test
            self.keyword = args.keyword

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
