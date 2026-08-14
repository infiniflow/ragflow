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

from unittest.mock import patch

from rag.svr.task_executor_refactor.dataset_wiki_generator import _validate_wiki_eligible_docs


def test_validate_wiki_eligible_docs_uses_template_model_without_pipeline():
    eligible = [
        ({"id": "doc-1", "pipeline_id": ""}, "template-1"),
        ({"id": "doc-2", "pipeline_id": ""}, "template-1"),
    ]

    with patch("api.db.services.compilation_template_service.CompilationTemplateService.get_saved") as get_saved:
        get_saved.return_value = {"config": {"llm_id": "template-chat"}}

        result = _validate_wiki_eligible_docs(eligible, "tenant-1")

    assert result == {"doc-1": "template-chat", "doc-2": "template-chat"}
    get_saved.assert_called_once_with("template-1", "tenant-1")


def test_validate_wiki_eligible_docs_uses_tenant_default_without_template_model():
    eligible = [({"id": "doc-1", "pipeline_id": ""}, "template-1")]

    with patch("api.db.services.compilation_template_service.CompilationTemplateService.get_saved") as get_saved:
        get_saved.return_value = {"config": {}}

        result = _validate_wiki_eligible_docs(eligible, "tenant-1")

    assert result == {"doc-1": None}
    get_saved.assert_called_once_with("template-1", "tenant-1")


def test_validate_wiki_eligible_docs_keeps_pipeline_model_resolution():
    eligible = [({"id": "doc-1", "pipeline_id": "pipeline-1"}, "template-1")]

    with patch("rag.svr.task_executor_refactor.dataset_wiki_generator._pipeline_compiler_llm_id", return_value="pipeline-chat") as pipeline_model:
        result = _validate_wiki_eligible_docs(eligible, "tenant-1")

    assert result == {"doc-1": "pipeline-chat"}
    pipeline_model.assert_called_once_with("pipeline-1")
