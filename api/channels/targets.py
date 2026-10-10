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

from __future__ import annotations

from typing import Any


def validate_agent_target(agent_id: str, tenant_id: str, *, service: Any = None) -> str | None:
    """Validate that a channel tenant may run the selected Agent."""
    if service is None:
        from api.db.services.canvas_service import UserCanvasService

        service = UserCanvasService

    found, agent = service.get_by_id(agent_id)
    if not found or getattr(agent, "canvas_category", "") != "agent_canvas":
        return "not_found"
    if not service.accessible(agent_id, tenant_id):
        return "forbidden"
    return None
