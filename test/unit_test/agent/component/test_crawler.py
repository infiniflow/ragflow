#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

import pytest

# Crawler imports the `crawl4ai` SDK at module load; skip where absent.
pytest.importorskip("crawl4ai")

from agent.tools.crawler import CrawlerParam  # noqa: E402


def test_param_instantiates():
    # Regression: CrawlerParam extends ToolParamBase, whose __init__ reads
    # self.meta["parameters"]. Without a `meta`, constructing the param raised
    # AttributeError, so any canvas containing a Crawler node failed to load.
    CrawlerParam()


def test_check_passes_with_defaults():
    CrawlerParam().check()


def test_meta_exposes_query_parameter():
    # The tool descriptor must advertise a required `query` parameter (the URL
    # to crawl) so an Agent's LLM can call it. `query` matches the frontend
    # form field and the {sys.query} convention shared by the other tools.
    meta = CrawlerParam().get_meta()
    params = meta["function"]["parameters"]
    assert "query" in params["properties"]
    assert "query" in params["required"]


def test_check_rejects_invalid_extract_type():
    param = CrawlerParam()
    param.extract_type = "pdf"
    with pytest.raises(ValueError):
        param.check()
