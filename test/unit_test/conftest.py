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

"""Shared setup for RAGFlow unit tests.

Several parsers and the chunking pipeline tokenize text with NLTK, which needs
the ``punkt_tab`` and ``wordnet`` data sets. Production provisions these via
``download_deps.py`` (into ``nltk_data``, exported as ``NLTK_DATA`` by
``docker/launch_backend_service.sh``) and ``api.validation`` at startup, but the
unit-test runner has neither. Without the data, tokenizer-backed tests such as
``test_epub_parser`` and ``test_dataflow_service`` fail with
``LookupError: Resource 'punkt_tab' not found``. Make sure the data is reachable
before any test imports a tokenizer: reuse a provisioned ``nltk_data`` directory
when present, and download only what is still missing.
"""

import os

import nltk

# Reuse data already fetched by download_deps.py so provisioned environments
# do not download it again. download_deps.py writes to a CWD-relative
# ``nltk_data/`` directory; when invoked from the repo root (the documented
# usage) that resolves to ``<repo_root>/nltk_data/``. A secondary location
# ``<repo_root>/ragflow_deps/nltk_data/`` is also checked for environments
# that run the script from inside ``ragflow_deps/``.
_REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), os.pardir, os.pardir))
for _nltk_candidate in (
    os.path.join(_REPO_ROOT, "ragflow_deps", "nltk_data"),
    os.path.join(_REPO_ROOT, "nltk_data"),
):
    if os.path.isdir(_nltk_candidate) and _nltk_candidate not in nltk.data.path:
        nltk.data.path.insert(0, _nltk_candidate)

# (download name, resource path used by nltk.data.find)
_REQUIRED_NLTK_DATA = (
    ("punkt_tab", "tokenizers/punkt_tab"),
    ("wordnet", "corpora/wordnet"),
)
for _name, _find_path in _REQUIRED_NLTK_DATA:
    try:
        nltk.data.find(_find_path)
    except LookupError:
        nltk.download(_name, quiet=True)
