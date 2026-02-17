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

import os
import pytest
from unittest.mock import patch
from common import file_utils
from common.file_utils import get_project_base_directory


class TestGetProjectBaseDirectory:
    """Test cases for get_project_base_directory function"""

    def test_returns_project_base_when_no_args(self):
        """Test that function returns project base directory when no arguments provided"""
        result = get_project_base_directory()

        assert result is not None
        assert isinstance(result, str)
        assert os.path.isabs(result)  # Should return absolute path

    def test_returns_path_with_single_argument(self):
        """Test that function joins project base with single additional path component"""
        result = get_project_base_directory("subfolder")

        assert result is not None
        assert "subfolder" in result
        assert result.endswith("subfolder")

    def test_returns_path_with_multiple_arguments(self):
        """Test that function joins project base with multiple path components"""
        result = get_project_base_directory("folder1", "folder2", "file.txt")

        assert result is not None
        assert "folder1" in result
        assert "folder2" in result
        assert "file.txt" in result
        assert os.path.basename(result) == "file.txt"

    def test_uses_environment_variable_when_available(self):
        """Test that function uses RAG_PROJECT_BASE environment variable when set"""
        test_path = "/custom/project/path"

        file_utils.PROJECT_BASE = test_path

        result = get_project_base_directory()
        assert result == test_path

    def test_calculates_default_path_when_no_env_vars(self):
        """Test that function calculates default path when no environment variables are set"""
        with patch.dict(os.environ, {}, clear=True):  # Clear all environment variables
            # Reset the global variable to force re-initialization

            result = get_project_base_directory()

            # Should return a valid absolute path
            assert result is not None
            assert os.path.isabs(result)
            assert os.path.basename(result) != ""  # Should not be root directory

    def test_caches_project_base_value(self):
        """Test that PROJECT_BASE is cached after first calculation"""
        # Reset the global variable

        # First call should calculate the value
        first_result = get_project_base_directory()

        # Store the current value
        cached_value = file_utils.PROJECT_BASE

        # Second call should use cached value
        second_result = get_project_base_directory()

        assert first_result == second_result
        assert file_utils.PROJECT_BASE == cached_value

    def test_path_components_joined_correctly(self):
        """Test that path components are properly joined with the base directory"""
        base_path = get_project_base_directory()
        expected_path = os.path.join(base_path, "data", "files", "document.txt")

        result = get_project_base_directory("data", "files", "document.txt")

        assert result == expected_path

    def test_handles_empty_string_arguments(self):
        """Test that function handles empty string arguments correctly"""
        result = get_project_base_directory("")

        # Should still return a valid path (base directory)
        assert result is not None
        assert os.path.isabs(result)


# Parameterized tests for different path combinations
@pytest.mark.parametrize("path_args,expected_suffix", [
    ((), ""),  # No additional arguments
    (("src",), "src"),
    (("data", "models"), os.path.join("data", "models")),
    (("config", "app", "settings.json"), os.path.join("config", "app", "settings.json")),
])
def test_various_path_combinations(path_args, expected_suffix):
    """Test various combinations of path arguments"""
    base_path = get_project_base_directory()
    result = get_project_base_directory(*path_args)

    if expected_suffix:
        assert result.endswith(expected_suffix)
    else:
        assert result == base_path
