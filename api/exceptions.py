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


class RAGFlowError(Exception):
    """Base class for all typed RAGFlow exceptions.

    Subclasses set ``http_status`` so the Quart error handler can produce a
    meaningful HTTP response without per-handler try/except blocks.
    """
    http_status: int = 500


class NotFoundError(RAGFlowError):
    """Resource does not exist (404)."""
    http_status = 404


class PermissionDeniedError(RAGFlowError):
    """Caller is authenticated but not authorised (403)."""
    http_status = 403


class ConflictError(RAGFlowError):
    """Uniqueness or state conflict (409)."""
    http_status = 409


class ValidationError(RAGFlowError):
    """Request payload or argument is invalid (422)."""
    http_status = 422


class ServiceUnavailableError(RAGFlowError):
    """Downstream dependency is unavailable (503)."""
    http_status = 503


class RetryableError(RAGFlowError):
    """Transient failure — task_executor should re-enqueue rather than mark permanently failed (503)."""
    http_status = 503
