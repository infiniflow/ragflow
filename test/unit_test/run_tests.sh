#!/bin/bash
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

# RAGFlow Unit Test Runner Script
# Usage: ./run_tests.sh [options]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Default options
COVERAGE=false
PARALLEL=false
VERBOSE=false
SPECIFIC_TEST=""
MARKERS=""

# Function to print colored output
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to show usage
show_usage() {
    cat << EOF
RAGFlow Unit Test Runner

Usage: $0 [OPTIONS]

OPTIONS:
    -h, --help              Show this help message
    -c, --coverage          Run tests with coverage report
    -p, --parallel          Run tests in parallel
    -v, --verbose           Verbose output
    -t, --test FILE         Run specific test file
    -m, --markers MARKERS   Run tests with specific markers (e.g., "unit", "integration")
    -f, --fast              Run only fast tests (exclude slow)
    -s, --services          Run only service tests
    -u, --utils             Run only utility tests

EXAMPLES:
    # Run all tests
    $0

    # Run with coverage
    $0 --coverage

    # Run in parallel
    $0 --parallel

    # Run specific test file
    $0 --test services/test_dialog_service.py

    # Run only unit tests
    $0 --markers unit

    # Run with coverage and parallel
    $0 --coverage --parallel

    # Run service tests only
    $0 --services

EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_usage
            exit 0
            ;;
        -c|--coverage)
            COVERAGE=true
            shift
            ;;
        -p|--parallel)
            PARALLEL=true
            shift
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -t|--test)
            SPECIFIC_TEST="$2"
            shift 2
            ;;
        -m|--markers)
            MARKERS="$2"
            shift 2
            ;;
        -f|--fast)
            MARKERS="not slow"
            shift
            ;;
        -s|--services)
            SPECIFIC_TEST="services/"
            shift
            ;;
        -u|--utils)
            SPECIFIC_TEST="common/"
            shift
            ;;
        *)
            print_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

# Check if pytest is installed
if ! command -v pytest &> /dev/null; then
    print_error "pytest is not installed"
    print_info "Install with: pip install pytest pytest-asyncio pytest-cov pytest-mock pytest-xdist"
    exit 1
fi

# Change to test directory
cd "$SCRIPT_DIR"

# Build pytest command
PYTEST_CMD="pytest"

# Add test path
if [ -n "$SPECIFIC_TEST" ]; then
    PYTEST_CMD="$PYTEST_CMD $SPECIFIC_TEST"
else
    PYTEST_CMD="$PYTEST_CMD ."
fi

# Add markers
if [ -n "$MARKERS" ]; then
    PYTEST_CMD="$PYTEST_CMD -m \"$MARKERS\""
fi

# Add verbose flag
if [ "$VERBOSE" = true ]; then
    PYTEST_CMD="$PYTEST_CMD -vv"
else
    PYTEST_CMD="$PYTEST_CMD -v"
fi

# Add coverage
if [ "$COVERAGE" = true ]; then
    PYTEST_CMD="$PYTEST_CMD --cov=../../api/db/services --cov-report=html --cov-report=term"
fi

# Add parallel execution
if [ "$PARALLEL" = true ]; then
    if ! python -c "import xdist" &> /dev/null; then
        print_warning "pytest-xdist not installed, running sequentially"
        print_info "Install with: pip install pytest-xdist"
    else
        PYTEST_CMD="$PYTEST_CMD -n auto"
    fi
fi

# Print test configuration
print_info "Running RAGFlow Unit Tests"
print_info "=========================="
print_info "Test Directory: $SCRIPT_DIR"
print_info "Coverage: $COVERAGE"
print_info "Parallel: $PARALLEL"
print_info "Verbose: $VERBOSE"
if [ -n "$SPECIFIC_TEST" ]; then
    print_info "Specific Test: $SPECIFIC_TEST"
fi
if [ -n "$MARKERS" ]; then
    print_info "Markers: $MARKERS"
fi
echo ""

# Run tests
print_info "Executing: $PYTEST_CMD"
echo ""

if eval "$PYTEST_CMD"; then
    echo ""
    print_success "All tests passed!"
    
    if [ "$COVERAGE" = true ]; then
        echo ""
        print_info "Coverage report generated in: $SCRIPT_DIR/htmlcov/index.html"
        print_info "Open with: open htmlcov/index.html (macOS) or xdg-open htmlcov/index.html (Linux)"
    fi
    
    exit 0
else
    echo ""
    print_error "Some tests failed!"
    exit 1
fi
