#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
"""Unit tests for the documentation assistant helper functions.

The tests extract the pure helper functions from
``api/apps/restful_apis/doc_assistant_api.py`` via AST so they can
run without pulling in the full Quart/DB dependency chain.
"""

from __future__ import annotations

import ast
import os
import re
import textwrap
from pathlib import Path

import pytest


REPO_ROOT = Path(__file__).resolve().parents[5]
DOC_ASSISTANT_SOURCE = REPO_ROOT / "api" / "apps" / "restful_apis" / "doc_assistant_api.py"


def _extract_functions(*func_names: str):
    """Extract named functions from doc_assistant_api.py without importing it."""
    source = DOC_ASSISTANT_SOURCE.read_text()
    tree = ast.parse(source)
    nodes = []
    for node in tree.body:
        if isinstance(node, ast.FunctionDef) and node.name in func_names:
            nodes.append(node)
    assert len(nodes) == len(func_names), f"Expected {len(func_names)} functions, found {len(nodes)}. Missing: {set(func_names) - {n.name for n in nodes}}"
    extracted = ast.Module(body=nodes, type_ignores=[])
    ns = {"re": re, "Path": Path, "os": os}
    exec(
        compile(extracted, str(DOC_ASSISTANT_SOURCE), "exec"),
        ns,
    )
    return tuple(ns[name] for name in func_names)


class TestSplitIntoSections:
    """Tests for _split_into_sections."""

    @pytest.fixture(autouse=True)
    def _setup(self):
        (self.split_into_sections, self.title_from_path) = _extract_functions("_split_into_sections", "_title_from_path")

    def test_split_headings(self):
        text = textwrap.dedent("""\
            # Getting Started

            This is the intro.

            ## Installation

            Install via Docker.

            ## Configuration

            Configure your settings.
        """)
        sections = self.split_into_sections(text, "quickstart.md")
        assert len(sections) == 3
        assert sections[0]["heading"] == "Getting Started"
        assert "intro" in sections[0]["content"]
        assert sections[1]["heading"] == "Installation"
        assert sections[2]["heading"] == "Configuration"
        for s in sections:
            assert s["source"] == "quickstart.md"

    def test_no_headings(self):
        text = "Just some plain text without headings."
        sections = self.split_into_sections(text, "readme.md")
        assert len(sections) == 1
        assert sections[0]["heading"] == "Readme"
        assert sections[0]["content"] == text

    def test_empty_text(self):
        sections = self.split_into_sections("", "empty.md")
        assert sections == []

    def test_content_truncation(self):
        text = "# Big Section\n\n" + "x" * 3000
        sections = self.split_into_sections(text, "big.md")
        assert len(sections) == 1
        assert len(sections[0]["content"]) <= 2000


class TestTitleFromPath:
    """Tests for _title_from_path."""

    @pytest.fixture(autouse=True)
    def _setup(self):
        (self.title_from_path,) = _extract_functions("_title_from_path")

    def test_simple_path(self):
        assert self.title_from_path("quickstart.md") == "Quickstart"

    def test_underscore_path(self):
        assert self.title_from_path("launch_ragflow_from_source.md") == "Launch Ragflow From Source"

    def test_hyphen_path(self):
        assert self.title_from_path("guides/agent/agent-introduction.md") == "Agent Introduction"

    def test_nested_path(self):
        result = self.title_from_path("administrator/configurations/configurations.md")
        assert result == "Configurations"


def _extract_search_docs_with_mock(mock_chunks):
    """Extract _search_docs with a _load_docs that returns mock_chunks."""
    source = DOC_ASSISTANT_SOURCE.read_text()
    tree = ast.parse(source)
    nodes = []
    for node in tree.body:
        if isinstance(node, ast.FunctionDef) and node.name == "_search_docs":
            nodes.append(node)
    assert len(nodes) == 1
    extracted = ast.Module(body=nodes, type_ignores=[])
    ns = {"re": re, "Path": Path, "os": os, "_load_docs": lambda: mock_chunks}
    exec(compile(extracted, str(DOC_ASSISTANT_SOURCE), "exec"), ns)
    return ns["_search_docs"]


class TestSearchDocs:
    """Tests for _search_docs using a mock doc cache."""

    def test_search_with_matching_terms(self):
        mock_chunks = [
            {"content": "How to install RAGFlow using Docker compose", "heading": "Installation", "source": "quickstart.md"},
            {"content": "Configure embedding models for your knowledge base", "heading": "Embedding Models", "source": "guides/models.md"},
            {"content": "Troubleshooting common errors in RAGFlow", "heading": "Troubleshooting", "source": "faq.md"},
        ]
        search_docs = _extract_search_docs_with_mock(mock_chunks)
        results = search_docs("install Docker")
        assert len(results) > 0
        assert any("install" in r["content"].lower() or "docker" in r["content"].lower() for r in results)

    def test_search_no_match(self):
        mock_chunks = [
            {"content": "Configure embedding models", "heading": "Models", "source": "guides/models.md"},
        ]
        search_docs = _extract_search_docs_with_mock(mock_chunks)
        results = search_docs("xyznonexistent")
        assert results == []

    def test_search_heading_boost(self):
        mock_chunks = [
            {"content": "Some text about docker", "heading": "Random", "source": "a.md"},
            {"content": "Some other text", "heading": "Docker Installation", "source": "b.md"},
        ]
        search_docs = _extract_search_docs_with_mock(mock_chunks)
        results = search_docs("docker")
        assert len(results) == 2
        # The heading-boosted result should come first
        assert results[0]["source"] == "b.md"


class TestBuildContext:
    """Tests for _build_context."""

    @pytest.fixture(autouse=True)
    def _setup(self):
        (self.build_context,) = _extract_functions("_build_context")

    def test_empty_chunks(self):
        result = self.build_context([])
        assert "No relevant documentation found" in result

    def test_multiple_chunks(self):
        chunks = [
            {"content": "Install via Docker.", "heading": "Installation", "source": "quickstart.md"},
            {"content": "Set your API key.", "heading": "Configuration", "source": "guides/config.md"},
        ]
        result = self.build_context(chunks)
        assert "Document 1" in result
        assert "Document 2" in result
        assert "quickstart.md" in result
        assert "guides/config.md" in result


class TestBuildReferences:
    """Tests for _build_references."""

    @pytest.fixture(autouse=True)
    def _setup(self):
        (self.build_references,) = _extract_functions("_build_references")

    def test_deduplicates_sources(self):
        chunks = [
            {"content": "a", "heading": "Section 1", "source": "quickstart.md"},
            {"content": "b", "heading": "Section 2", "source": "quickstart.md"},
            {"content": "c", "heading": "FAQ", "source": "faq.md"},
        ]
        refs = self.build_references(chunks)
        assert len(refs) == 2
        sources = [r["source"] for r in refs]
        assert "quickstart.md" in sources
        assert "faq.md" in sources

    def test_url_generation(self):
        chunks = [
            {"content": "x", "heading": "Title", "source": "guides/agent/agent_introduction.md"},
        ]
        refs = self.build_references(chunks)
        assert len(refs) == 1
        assert refs[0]["url"] == "https://ragflow.io/docs/dev/guides/agent/agent_introduction"

    def test_mdx_extension_stripped(self):
        chunks = [
            {"content": "x", "heading": "Title", "source": "quickstart.mdx"},
        ]
        refs = self.build_references(chunks)
        assert refs[0]["url"] == "https://ragflow.io/docs/dev/quickstart"

    def test_empty_chunks(self):
        assert self.build_references([]) == []
