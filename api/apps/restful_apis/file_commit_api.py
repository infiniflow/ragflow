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

import logging
from functools import wraps

from quart import request

from api.apps import login_required, current_user
from api.utils.api_utils import get_json_result, get_data_error_result, get_request_json, server_error_response, validate_request

# manager is injected dynamically by api.apps.register_page() before this
# module is exec'd. DO NOT assign manager = None here — it would overwrite
# the Blueprint that register_page set on the module.
from api.db.services.file_commit_service import FileCommitService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.file_service import FileService
from common.constants import FileSource

logger = logging.getLogger(__name__)

_ENTITY_RESOLVERS = {}

# Counter to give each generated route function a unique name,
# preventing Quart Blueprint endpoint name collisions.
_route_suffix = [0]


def _register_resolver(entity_type):
    """Decorator that registers a folder_id resolver for an entity type.

    The decorated function receives (entity_id) and must return a folder_id
    or None if the entity has no corresponding folder.
    """
    def decorator(func):
        _ENTITY_RESOLVERS[entity_type] = func
        @wraps(func)
        def wrapper(entity_id):
            return func(entity_id)
        return wrapper
    return decorator


def _resolve_folder_id(entity_type, entity_id):
    """Resolve an entity (dataset/memory/skill) to its folder_id."""
    resolver = _ENTITY_RESOLVERS.get(entity_type)
    if resolver is None:
        return None
    return resolver(entity_id)


@_register_resolver("datasets")
def _resolve_dataset_folder(dataset_id):
    success, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not success:
        return None
    # Find the folder with matching name, source_type, and tenant_id
    folders = FileService.query(
        name=kb.name,
        source_type=FileSource.KNOWLEDGEBASE.value,
        type="folder",
        tenant_id=kb.tenant_id,
    )
    if folders:
        return folders[0].id
    return None


# ── Route registration helper ─────────────────────────────────────────────

def _register_commit_routes(prefix, param_name, resolver_type=None):
    """Register all 8 commit endpoints for a given URL prefix.

    Args:
        prefix: URL prefix like '/folders/<folder_id>'
        param_name: The URL parameter name (e.g. 'folder_id', 'dataset_id')
        resolver_type: If set, resolve param_name → folder_id before calling logic
    """
    # Unique suffix for this call to prevent Blueprint endpoint name collisions
    _route_suffix[0] += 1
    _n = _route_suffix[0]

    def _resolve(entity_id):
        if resolver_type is None:
            return entity_id  # already a folder_id
        folder_id = _resolve_folder_id(resolver_type, entity_id)
        if folder_id is None:
            raise ValueError(f"Could not resolve {resolver_type} '{entity_id}' to a folder")
        return folder_id

    # ── Create commit ──────────────────────────────────────────────────────
    @manager.route(f'{prefix}/commits', methods=['POST'], endpoint=f'create_commit_{_n}')  # noqa: F821
    @login_required
    @validate_request("message", "files")
    async def create_commit(entity_id):
        folder_id = _resolve(entity_id)
        req = await get_request_json()
        try:
            commit = FileCommitService.create_commit(
                folder_id=folder_id,
                author_id=current_user.id,
                message=req["message"],
                file_changes=req["files"],
            )
            return get_json_result(data={
                "id": commit.id,
                "folder_id": commit.folder_id,
                "parent_id": commit.parent_id,
                "message": commit.message,
                "author_id": commit.author_id,
                "file_count": commit.file_count,
                "tree_state": commit.tree_state,
                "create_time": commit.create_time,
            })
        except Exception as e:
            return server_error_response(e)

    # ── List commits ───────────────────────────────────────────────────────
    @manager.route(f'{prefix}/commits', methods=['GET'], endpoint=f'list_commits_{_n}')  # noqa: F821
    @login_required
    async def list_commits(entity_id):
        folder_id = _resolve(entity_id)
        try:
            page = int(request.args.get("page", 1))
            page_size = int(request.args.get("page_size", 15))
            order_by = request.args.get("order_by", "create_time")
            desc = request.args.get("desc", "true").lower() != "false"
            commits, total = FileCommitService.list_commits(folder_id, page, page_size, order_by, desc)
            return get_json_result(data={
                "total": total,
                "page": page,
                "page_size": page_size,
                "commits": [{
                    "id": c.id,
                    "folder_id": c.folder_id,
                    "parent_id": c.parent_id,
                    "message": c.message,
                    "author_id": c.author_id,
                    "file_count": c.file_count,
                    "create_time": c.create_time,
                } for c in commits],
            })
        except Exception as e:
            return server_error_response(e)

    # ── Get commit ─────────────────────────────────────────────────────────
    @manager.route(f'{prefix}/commits/<commit_id>', methods=['GET'], endpoint=f'get_commit_{_n}')  # noqa: F821
    @login_required
    async def get_commit(entity_id, commit_id):
        folder_id = _resolve(entity_id)
        try:
            commit = FileCommitService.get_commit(commit_id)
            if not commit:
                return get_data_error_result("Commit not found")
            if commit.folder_id != folder_id:
                return get_data_error_result("Commit not found in workspace")
            items = FileCommitService.list_commit_files(commit_id)
            return get_json_result(data={
                "id": commit.id,
                "folder_id": commit.folder_id,
                "parent_id": commit.parent_id,
                "message": commit.message,
                "author_id": commit.author_id,
                "file_count": commit.file_count,
                "create_time": commit.create_time,
                "files": [{
                    "file_id": item.file_id,
                    "operation": item.operation,
                    "old_hash": item.old_hash,
                    "new_hash": item.new_hash,
                    "old_name": item.old_name,
                    "new_name": item.new_name,
                } for item in items],
            })
        except Exception as e:
            return server_error_response(e)

    # ── List commit files ──────────────────────────────────────────────────
    @manager.route(f'{prefix}/commits/<commit_id>/files', methods=['GET'], endpoint=f'list_commit_files_{_n}')  # noqa: F821
    @login_required
    async def list_commit_files(entity_id, commit_id):
        folder_id = _resolve(entity_id)
        try:
            commit = FileCommitService.get_commit(commit_id)
            if not commit:
                return get_data_error_result("Commit not found")
            if commit.folder_id != folder_id:
                return get_data_error_result("Commit not found in workspace")
            items = FileCommitService.list_commit_files(commit_id)
            return get_json_result(data=[{
                "id": item.id,
                "file_id": item.file_id,
                "operation": item.operation,
                "old_hash": item.old_hash,
                "new_hash": item.new_hash,
                "old_location": item.old_location,
                "new_location": item.new_location,
                "old_name": item.old_name,
                "new_name": item.new_name,
            } for item in items])
        except Exception as e:
            return server_error_response(e)

    # ── Diff commits ───────────────────────────────────────────────────────
    @manager.route(f'{prefix}/commits/diff', methods=['GET'], endpoint=f'diff_commits_{_n}')  # noqa: F821
    @login_required
    async def diff_commits(entity_id):
        folder_id = _resolve(entity_id)
        from_id = request.args.get("from")
        to_id = request.args.get("to")
        if not from_id or not to_id:
            return get_data_error_result("'from' and 'to' parameters are required")
        try:
            from_commit = FileCommitService.get_commit(from_id)
            to_commit = FileCommitService.get_commit(to_id)
            if not from_commit or not to_commit:
                return get_data_error_result("Commit not found")
            if from_commit.folder_id != folder_id or to_commit.folder_id != folder_id:
                return get_data_error_result("Commit not found in workspace")
            diff = FileCommitService.diff_commits(from_id, to_id)
            return get_json_result(data=diff)
        except Exception as e:
            return server_error_response(e)

    # ── Get uncommitted changes ────────────────────────────────────────────
    @manager.route(f'{prefix}/changes', methods=['GET'], endpoint=f'get_uncommitted_changes_{_n}')  # noqa: F821
    @login_required
    async def get_uncommitted_changes(entity_id):
        folder_id = _resolve(entity_id)
        try:
            changes = FileCommitService.get_uncommitted_changes(folder_id)
            return get_json_result(data=changes)
        except Exception as e:
            return server_error_response(e)

    # ── Get commit tree ────────────────────────────────────────────────────
    @manager.route(f'{prefix}/commits/<commit_id>/tree', methods=['GET'], endpoint=f'get_commit_tree_{_n}')  # noqa: F821
    @login_required
    async def get_commit_tree(entity_id, commit_id):
        folder_id = _resolve(entity_id)
        try:
            commit = FileCommitService.get_commit(commit_id)
            if not commit:
                return get_data_error_result("Commit not found")
            if commit.folder_id != folder_id:
                return get_data_error_result("Commit not found in workspace")
            tree = FileCommitService.get_commit_tree(commit_id)
            return get_json_result(data=tree)
        except Exception as e:
            return server_error_response(e)

    # ── Get commit file content ────────────────────────────────────────────
    @manager.route(f'{prefix}/commits/<commit_id>/files/<file_id>/content', methods=['GET'], endpoint=f'get_commit_file_content_{_n}')  # noqa: F821
    @login_required
    async def get_commit_file_content(entity_id, commit_id, file_id):
        folder_id = _resolve(entity_id)
        try:
            commit = FileCommitService.get_commit(commit_id)
            if not commit:
                return get_data_error_result("Commit not found")
            if commit.folder_id != folder_id:
                return get_data_error_result("Commit not found in workspace")
            content = FileCommitService.get_commit_file_content(folder_id, commit_id, file_id)
            if content is None:
                return get_data_error_result("File not found in this commit")
            return get_json_result(data={"content": content.decode("utf-8", errors="replace")})
        except Exception as e:
            return server_error_response(e)

    # Expose handlers at module level for direct testing.
    _g = globals()
    _g['create_commit'] = create_commit
    _g['list_commits'] = list_commits
    _g['get_commit'] = get_commit
    _g['list_commit_files'] = list_commit_files
    _g['diff_commits'] = diff_commits
    _g['get_uncommitted_changes'] = get_uncommitted_changes
    _g['get_commit_tree'] = get_commit_tree
    _g['get_commit_file_content'] = get_commit_file_content

# ── Register routes for all entity types ──────────────────────────────────
# All URL patterns use <entity_id> as the consistent param name.
# For /folders/ entity_id IS the folder_id directly.
# For other entity types entity_id is resolved via _resolve_folder_id().
# Register datasets first, workspace second, folders last —
# the last call's handlers overwrite module-level names for test access.
_register_commit_routes('/datasets/<entity_id>', 'entity_id', resolver_type='datasets')
_register_commit_routes('/workspace/<entity_id>', 'entity_id')  # alias — workspace_id == folder_id
_register_commit_routes('/folders/<entity_id>', 'entity_id')  # direct — entity_id == folder_id (wins)
# /memories and /skills routes are not mounted until resolvers are implemented.


# ── File version history (shared across all entity types) ─────────────────
@manager.route('/files/<file_id>/versions', methods=['GET'])  # noqa: F821
@login_required
async def get_file_version_history(file_id):
    try:
        versions = FileCommitService.get_file_version_history(file_id)
        return get_json_result(data=versions)
    except Exception as e:
        return server_error_response(e)
