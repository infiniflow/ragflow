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

# NLTK >=3.10 refuses proxied downloads (SSRF guard, CWE-918) by default. The
# CI runners and some dev boxes sit behind a proxy, so opt in to proxied fetches
# before importing nltk; otherwise `nltk.download` fails with a Security
# Violation. Must be set before the `import nltk` below so pathsec reads it.
os.environ.setdefault("NLTK_ALLOW_PROXIED_URLOPEN", "1")

import nltk
import warnings

# Reuse data already fetched by download_deps.py (the directory the app exports
# as NLTK_DATA) so provisioned environments do not download it again. Create it
# if absent so the fallback download below lands in a repo-local, reproducible
# location instead of a shared home directory.
_REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), os.pardir, os.pardir))
_LOCAL_NLTK_DATA = os.path.join(_REPO_ROOT, "ragflow_deps", "nltk_data")
os.makedirs(_LOCAL_NLTK_DATA, exist_ok=True)
if _LOCAL_NLTK_DATA not in nltk.data.path:
    nltk.data.path.insert(0, _LOCAL_NLTK_DATA)

# (download name, resource path used by nltk.data.find)
_REQUIRED_NLTK_DATA = (
    ("punkt_tab", "tokenizers/punkt_tab"),
    ("punkt", "tokenizers/punkt"),
    ("wordnet", "corpora/wordnet"),
)
for _name, _find_path in _REQUIRED_NLTK_DATA:
    try:
        nltk.data.find(_find_path)
    except LookupError:
        # On shared CI runners the download dir is often group-writable, which
        # makes NLTK emit a "non-private download directory" UserWarning. pytest
        # escalates warnings to errors (filterwarnings = error), so suppress it
        # for the duration of the download and fetch into the repo-local dir.
        with warnings.catch_warnings():
            warnings.simplefilter("ignore", UserWarning)
            nltk.download(_name, download_dir=_LOCAL_NLTK_DATA, quiet=True)
