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

import hashlib
import json
import logging

from api.db.db_models import DB, FileCommit, FileCommitItem, File
from api.db.services.common_service import CommonService
from api.db.services.file_service import FileService
from common import settings
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp, datetime_format

logger = logging.getLogger(__name__)


class FileCommitService(CommonService):
    model = FileCommit

    @classmethod
    @DB.connection_context()
    def create_commit(cls, folder_id, author_id, message, file_changes):
        """Create a new commit for a workspace folder.

        Args:
            folder_id: The workspace folder ID
            author_id: The user ID
            message: Commit message
            file_changes: List of dicts:
                [{"file_id": str, "file_name": str, "operation": "add"|"modify"|"delete"|"rename",
                  "content": str (optional, for add/modify), "content_hash": str (optional),
                  "old_name": str, "new_name": str (for rename)}]

        Returns:
            The created FileCommit instance
        """
        # 1. Get the latest (chain head) commit for this folder
        latest_commit = cls._get_latest_commit(folder_id)

        # 2. Begin creating the commit record
        commit_id = get_uuid()
        now_ts = current_timestamp()
        now_dt = datetime_format()

        commit_data = {
            "id": commit_id,
            "folder_id": folder_id,
            "parent_id": latest_commit.id if latest_commit else None,
            "message": message,
            "author_id": author_id,
            "file_count": len(file_changes),
            "create_time": now_ts,
            "create_date": now_dt,
            "update_time": now_ts,
            "update_date": now_dt,
        }

        with DB.atomic():
            # 3. Insert commit record
            FileCommit(**commit_data).save(force_insert=True)

            # 4. Build new tree state and process each file change
            tree_state = {}
            if latest_commit and latest_commit.tree_state:
                try:
                    tree_state = json.loads(latest_commit.tree_state)
                except (json.JSONDecodeError, TypeError):
                    tree_state = {}

            storage_impl = settings.STORAGE_IMPL

            for change in file_changes:
                op = change.get("operation", "modify")
                file_id = change.get("file_id", "")
                commit_item_id = get_uuid()

                item = {
                    "id": commit_item_id,
                    "commit_id": commit_id,
                    "file_id": file_id,
                    "operation": op,
                    "create_time": now_ts,
                    "create_date": now_dt,
                    "update_time": now_ts,
                    "update_date": now_dt,
                }

                if op == "add":
                    content = change.get("content", "")
                    content_bytes = content.encode("utf-8") if isinstance(content, str) else content
                    content_hash = hashlib.sha256(content_bytes).hexdigest()
                    obj_key = f".objects/{content_hash}"

                    # Store blob via content-addressable storage
                    if storage_impl:
                        storage_impl.put(folder_id, obj_key, content_bytes)

                    item["new_hash"] = content_hash
                    item["new_location"] = obj_key

                    # Update file record in DB
                    FileService.update_by_id(file_id, {
                        "location": obj_key,
                        "size": len(content_bytes),
                    })

                    # Update tree state
                    tree_state[file_id] = {
                        "hash": content_hash,
                        "location": obj_key,
                        "name": change.get("file_name", ""),
                        "size": len(content_bytes),
                        "status": "1",
                    }

                elif op == "modify":
                    content = change.get("content", "")
                    content_bytes = content.encode("utf-8") if isinstance(content, str) else content
                    content_hash = hashlib.sha256(content_bytes).hexdigest()
                    obj_key = f".objects/{content_hash}"

                    # Record old hash
                    old_entry = tree_state.get(file_id, {})
                    old_hash = old_entry.get("hash", "")
                    old_location = old_entry.get("location", "")

                    if old_hash:
                        item["old_hash"] = old_hash
                        item["old_location"] = old_location

                    # Store new blob
                    if storage_impl:
                        storage_impl.put(folder_id, obj_key, content_bytes)

                    item["new_hash"] = content_hash
                    item["new_location"] = obj_key

                    # Update file record
                    FileService.update_by_id(file_id, {
                        "location": obj_key,
                        "size": len(content_bytes),
                    })

                    # Update tree state
                    tree_state[file_id] = {
                        "hash": content_hash,
                        "location": obj_key,
                        "name": change.get("file_name", tree_state.get(file_id, {}).get("name", "")),
                        "size": len(content_bytes),
                        "status": "1",
                    }

                elif op == "delete":
                    old_entry = tree_state.get(file_id, {})
                    old_hash = old_entry.get("hash", "")
                    old_location = old_entry.get("location", "")
                    if old_hash:
                        item["old_hash"] = old_hash
                        item["old_location"] = old_location

                    # Soft-delete the file record
                    FileService.model.update(status="0").where(
                        FileService.model.id == file_id
                    ).execute()

                    # Remove from tree state (mark deleted)
                    if file_id in tree_state:
                        tree_state[file_id]["status"] = "0"

                elif op == "rename":
                    old_name = change.get("old_name", "")
                    new_name = change.get("new_name", "")
                    item["old_name"] = old_name
                    item["new_name"] = new_name

                    # Update the file record name
                    FileService.update_by_id(file_id, {"name": new_name})

                    # Update tree state
                    if file_id in tree_state:
                        tree_state[file_id]["name"] = new_name

                # Insert commit item
                FileCommitItem(**item).save(force_insert=True)

            # 5. Save the tree state snapshot
            tree_json = json.dumps(tree_state, ensure_ascii=False)
            cls.model.update(tree_state=tree_json).where(cls.model.id == commit_id).execute()

        return cls.get_by_id(commit_id)

    @classmethod
    @DB.connection_context()
    def _get_latest_commit(cls, folder_id):
        """Get the latest (chain head) commit for a folder."""
        try:
            return cls.model.select().where(
                cls.model.folder_id == folder_id
            ).order_by(cls.model.create_time.desc()).first()
        except Exception:
            return None

    @classmethod
    @DB.connection_context()
    def list_commits(cls, folder_id, page=1, page_size=15, order_by="create_time", desc=True):
        """List commits for a workspace folder with pagination."""
        total = cls.model.select().where(cls.model.folder_id == folder_id).count()

        query = cls.model.select().where(cls.model.folder_id == folder_id)
        if desc:
            query = query.order_by(getattr(cls.model, order_by).desc())
        else:
            query = query.order_by(getattr(cls.model, order_by).asc())

        if page and page_size:
            offset = (page - 1) * page_size
            query = query.offset(offset).limit(page_size)

        return list(query), total

    @classmethod
    @DB.connection_context()
    def get_commit(cls, commit_id):
        """Get a single commit by ID."""
        success, commit = cls.get_by_id(commit_id)
        return commit if success else None

    @classmethod
    @DB.connection_context()
    def list_commit_files(cls, commit_id):
        """List all file change items for a commit."""
        items = FileCommitItem.select().where(FileCommitItem.commit_id == commit_id)
        return list(items)

    @classmethod
    @DB.connection_context()
    def diff_commits(cls, from_id, to_id):
        """Compare two commits and return the diff.

        Returns list of dicts with fields:
            file_id, file_name, operation, old_hash, new_hash, old_location, new_location
        """
        from_items = {item.file_id: item for item in FileCommitItem.select().where(
            FileCommitItem.commit_id == from_id)}
        to_items = {item.file_id: item for item in FileCommitItem.select().where(
            FileCommitItem.commit_id == to_id)}

        # Get latest tree_state for file names (use the 'to' commit)
        _, to_commit = cls.get_by_id(to_id)
        tree_state = {}
        if to_commit and to_commit.tree_state:
            try:
                tree_state = json.loads(to_commit.tree_state)
            except Exception:
                pass

        diff = []
        all_file_ids = set(from_items.keys()) | set(to_items.keys())
        for fid in all_file_ids:
            from_item = from_items.get(fid)
            to_item = to_items.get(fid)

            if from_item and not to_item:
                # File deleted between from and to (present in from, absent in to)
                # But it may have been deleted in an intermediate commit
                # Let's check: if it's in from_items but not in to_items, it was deleted
                diff.append({
                    "file_id": fid,
                    "file_name": tree_state.get(fid, {}).get("name", ""),
                    "operation": "delete",
                    "old_hash": from_item.new_hash,
                    "old_location": from_item.new_location,
                    "new_hash": None,
                    "new_location": None,
                })
            elif not from_item and to_item:
                # File added
                diff.append({
                    "file_id": fid,
                    "file_name": tree_state.get(fid, {}).get("name", ""),
                    "operation": "add",
                    "old_hash": None,
                    "old_location": None,
                    "new_hash": to_item.new_hash,
                    "new_location": to_item.new_location,
                })
            else:
                # Both exist - compare hashes
                if from_item.new_hash != to_item.new_hash or from_item.operation != to_item.operation:
                    diff.append({
                        "file_id": fid,
                        "file_name": tree_state.get(fid, {}).get("name", ""),
                        "operation": to_item.operation if to_item else "modify",
                        "old_hash": from_item.new_hash,
                        "old_location": from_item.new_location,
                        "new_hash": to_item.new_hash if to_item else None,
                        "new_location": to_item.new_location if to_item else None,
                    })

        return diff

    @classmethod
    @DB.connection_context()
    def get_uncommitted_changes(cls, folder_id):
        """Get uncommitted changes by comparing current File table with latest commit.

        Returns list of dicts: [{"file_id", "file_name", "operation": "add"|"modify"|"delete"}]
        """
        # Get latest commit's tree state
        latest = cls._get_latest_commit(folder_id)
        committed_files = {}
        if latest and latest.tree_state:
            try:
                committed_files = json.loads(latest.tree_state)
            except Exception:
                pass

        # Get all current (live) files in this folder
        current_files = {}
        try:
            files_query = File.select().where(
                File.parent_id == folder_id,
                File.id != folder_id,
            )
            for f in files_query:
                current_files[f.id] = f
        except Exception:
            pass

        changes = []
        processed = set()

        # Check for modified and deleted files
        for fid, committed_entry in committed_files.items():
            processed.add(fid)
            if committed_entry.get("status") == "0":
                # Already deleted in commit — skip
                continue

            if fid in current_files:
                live_file = current_files[fid]
                # File still exists — check if hash differs
                live_hash = _compute_file_hash(folder_id, fid)
                committed_hash = committed_entry.get("hash", "")
                if live_hash and live_hash != committed_hash:
                    changes.append({
                        "file_id": fid,
                        "file_name": committed_entry.get("name", ""),
                        "operation": "modify",
                    })
            else:
                # File no longer in live table — deleted
                if FileService.get_or_none(id=fid) is None:
                    changes.append({
                        "file_id": fid,
                        "file_name": committed_entry.get("name", ""),
                        "operation": "delete",
                    })

        # Check for newly added files
        for fid, live_file in current_files.items():
            if fid not in processed:
                changes.append({
                    "file_id": fid,
                    "file_name": live_file.name,
                    "operation": "add",
                })

        return changes

    @classmethod
    @DB.connection_context()
    def get_commit_tree(cls, commit_id):
        """Get the tree state snapshot for a commit."""
        success, commit = cls.get_by_id(commit_id)
        if not success or not commit.tree_state:
            return {}
        try:
            return json.loads(commit.tree_state)
        except Exception:
            return {}

    @classmethod
    @DB.connection_context()
    def get_commit_file_content(cls, folder_id, commit_id, file_id):
        """Get file content as it existed in a given commit."""
        success, commit = cls.get_by_id(commit_id)
        if not success:
            return None

        # Read from commit item
        item = FileCommitItem.select().where(
            FileCommitItem.commit_id == commit_id,
            FileCommitItem.file_id == file_id,
        ).first()
        if not item:
            return None

        obj_key = item.new_hash
        if not obj_key:
            return None

        obj_path = f".objects/{obj_key}"
        storage_impl = settings.STORAGE_IMPL
        if not storage_impl:
            return None

        return storage_impl.get(folder_id, obj_path)

    @classmethod
    @DB.connection_context()
    def get_file_version_history(cls, file_id):
        """Get version history for a specific file across all commits.

        Returns list of dicts: [{"commit_id", "operation", "hash", "create_time", "message"}]
        """
        items = FileCommitItem.select().where(FileCommitItem.file_id == file_id).order_by(
            FileCommitItem.create_time.desc())

        versions = []
        for item in items:
            commit = cls.get_commit(item.commit_id)
            if commit:
                versions.append({
                    "commit_id": item.commit_id,
                    "operation": item.operation,
                    "hash": item.new_hash or item.old_hash or "",
                    "create_time": item.create_time,
                    "message": commit.message,
                })

        return versions


def _compute_file_hash(folder_id, file_id):
    """Compute SHA256 hash of current file content from storage."""
    try:
        file_record = FileService.get_by_id(file_id)
        if not file_record[0]:
            return None
        file = file_record[1]
        if not file.location:
            return None

        storage = settings.STORAGE_IMPL
        if not storage:
            return None

        data = storage.get(folder_id, file.location)
        if data:
            return hashlib.sha256(data).hexdigest()
        return None
    except Exception:
        return None
