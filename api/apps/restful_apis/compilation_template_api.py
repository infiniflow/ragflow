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

from quart import Response

from api.apps import current_user, login_required
from api.apps.restful_apis.utils.compilation_template_validation import validate_template_payload
from api.db.services.compilation_template_service import CompilationTemplateService
from api.utils.api_utils import get_json_result, server_error_response


_validate_template_payload = validate_template_payload


@manager.route("/compilation_templates/builtins", methods=["GET"])  # noqa: F821
@login_required
def list_builtin_templates() -> Response:
    """Built-in template palette — used as the per-child pre-fill in the
    "Add template group" panel. Groups themselves are always user-created;
    no builtin groups exist.
    """
    try:
        templates = CompilationTemplateService.list_builtins()
        if not templates:
            CompilationTemplateService.seed_builtins_from_files()
            templates = CompilationTemplateService.list_builtins()
        if not templates:
            templates = [
                {
                    "id": template["id"],
                    "kind": template["kind"],
                    "display_name": template["name"],
                    "description": template.get("description", ""),
                    "config": template["config"],
                }
                for template in CompilationTemplateService.load_builtins_from_files()
            ]
        templates = CompilationTemplateService.fill_default_llm_for_templates(templates, current_user.id)
        return get_json_result(data=templates)
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/compilation_templates/wiki_presets", methods=["GET"])  # noqa: F821
@login_required
def list_wiki_presets() -> Response:
    """Wiki page-structure presets loaded from
    ``api/db/init_data/compilation_templates/wiki/*.yaml``.

    Each entry carries ``id`` (filename stem) + ``topic`` +
    ``instruction`` + ``page_example`` so the artifact-template editor
    can pre-fill its "Page-structure example" / "Global rules" fields
    from a canned skeleton. Filesystem-fresh per request; no DB seed.
    """
    try:
        presets = CompilationTemplateService.load_wiki_presets_from_files()
        return get_json_result(data=presets)
    except Exception as exc:
        return server_error_response(exc)
