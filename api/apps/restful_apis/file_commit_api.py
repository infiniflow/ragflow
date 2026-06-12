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

from quart import request

from api.apps import login_required, current_user
from api.utils.api_utils import get_json_result, get_data_error_result, get_request_json, server_error_response, validate_request
from api.db.services.file_commit_service import FileCommitService

logger = logging.getLogger(__name__)


@manager.route('/workspaces/<folder_id>/commits', methods=['POST'])
@login_required
@validate_request("message", "files")
async def create_commit(folder_id):
    """Create a new commit for a workspace folder.

    Request body:
    {
        "message": "commit message",
        "files": [
            {
                "file_id": "...",
                "file_name": "...",
                "operation": "add|modify|delete|rename",
                "content": "..." (for add/modify),
                "old_name": "..." (for rename),
                "new_name": "..." (for rename)
            }
        ]
    }
    """
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


@manager.route('/workspaces/<folder_id>/commits', methods=['GET'])
@login_required
async def list_commits(folder_id):
    """List all commits for a workspace folder (paginated)."""
    try:
        page = int(request.args.get("page", 1))
        page_size = int(request.args.get("page_size", 15))
        order_by = request.args.get("order_by", "create_time")
        desc = request.args.get("desc", "true").lower() != "false"

        commits, total = FileCommitService.list_commits(folder_id, page, page_size, order_by, desc)

        commit_list = []
        for c in commits:
            commit_list.append({
                "id": c.id,
                "folder_id": c.folder_id,
                "parent_id": c.parent_id,
                "message": c.message,
                "author_id": c.author_id,
                "file_count": c.file_count,
                "create_time": c.create_time,
            })

        return get_json_result(data={
            "total": total,
            "page": page,
            "page_size": page_size,
            "commits": commit_list,
        })
    except Exception as e:
        return server_error_response(e)


@manager.route('/workspaces/<folder_id>/commits/<commit_id>', methods=['GET'])
@login_required
async def get_commit(folder_id, commit_id):
    """Get details of a single commit."""
    try:
        commit = FileCommitService.get_commit(commit_id)
        if not commit:
            return get_data_error_result("Commit not found")
        items = FileCommitService.list_commit_files(commit_id)
        return get_json_result(data={
            "id": commit.id,
            "folder_id": commit.folder_id,
            "parent_id": commit.parent_id,
            "message": commit.message,
            "author_id": commit.author_id,
            "file_count": commit.file_count,
            "create_time": commit.create_time,
            "files": [
                {
                    "file_id": item.file_id,
                    "operation": item.operation,
                    "old_hash": item.old_hash,
                    "new_hash": item.new_hash,
                    "old_name": item.old_name,
                    "new_name": item.new_name,
                }
                for item in items
            ],
        })
    except Exception as e:
        return server_error_response(e)


@manager.route('/workspaces/<folder_id>/commits/<commit_id>/files', methods=['GET'])
@login_required
async def list_commit_files(folder_id, commit_id):
    """List all file changes in a commit."""
    try:
        items = FileCommitService.list_commit_files(commit_id)
        return get_json_result(data=[
            {
                "id": item.id,
                "file_id": item.file_id,
                "operation": item.operation,
                "old_hash": item.old_hash,
                "new_hash": item.new_hash,
                "old_location": item.old_location,
                "new_location": item.new_location,
                "old_name": item.old_name,
                "new_name": item.new_name,
            }
            for item in items
        ])
    except Exception as e:
        return server_error_response(e)


@manager.route('/workspaces/<folder_id>/commits/diff', methods=['GET'])
@login_required
async def diff_commits(folder_id):
    """Compare two commits.

    Query params: from (commit_id), to (commit_id)
    """
    from_id = request.args.get("from")
    to_id = request.args.get("to")
    if not from_id or not to_id:
        return get_data_error_result("'from' and 'to' parameters are required")

    try:
        diff = FileCommitService.diff_commits(from_id, to_id)
        return get_json_result(data=diff)
    except Exception as e:
        return server_error_response(e)


@manager.route('/workspaces/<folder_id>/changes', methods=['GET'])
@login_required
async def get_uncommitted_changes(folder_id):
    """Get uncommitted changes (like git status)."""
    try:
        changes = FileCommitService.get_uncommitted_changes(folder_id)
        return get_json_result(data=changes)
    except Exception as e:
        return server_error_response(e)


@manager.route('/workspaces/<folder_id>/commits/<commit_id>/tree', methods=['GET'])
@login_required
async def get_commit_tree(folder_id, commit_id):
    """Get the folder tree snapshot for a commit."""
    try:
        tree = FileCommitService.get_commit_tree(commit_id)
        return get_json_result(data=tree)
    except Exception as e:
        return server_error_response(e)


@manager.route('/workspaces/<folder_id>/commits/<commit_id>/files/<file_id>/content', methods=['GET'])
@login_required
async def get_commit_file_content(folder_id, commit_id, file_id):
    """Get file content as it existed in a specific commit."""
    try:
        content = FileCommitService.get_commit_file_content(folder_id, commit_id, file_id)
        if content is None:
            return get_data_error_result("File not found in this commit")
        return get_json_result(data={"content": content.decode("utf-8", errors="replace")})
    except Exception as e:
        return server_error_response(e)


@manager.route('/files/<file_id>/versions', methods=['GET'])
@login_required
async def get_file_version_history(file_id):
    """Get version history for a specific file."""
    try:
        versions = FileCommitService.get_file_version_history(file_id)
        return get_json_result(data=versions)
    except Exception as e:
        return server_error_response(e)
