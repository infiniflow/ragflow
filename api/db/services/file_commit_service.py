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

import datetime
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


def _get_file_parent_id(file_id):
    """Look up a file's parent_id from the File table."""
    try:
        row = File.get_or_none(File.id == file_id)
        if row:
            return row.parent_id
    except Exception:
        pass
    return None


def _collect_all_files_under(folder_id):
    """Recursively collect all non-folder files under a folder (including sub-folders).

    Returns a dict of {file_id: File_model_instance}.
    """
    results = {}
    try:
        # Direct file children (non-folder) of this folder
        for f in File.select().where(
            File.parent_id == folder_id,
            File.id != folder_id,
            File.type != "folder",
        ):
            results[f.id] = f
        # Sub-folders — recurse
        for sub in File.select().where(
            File.parent_id == folder_id,
            File.type == "folder",
        ):
            results.update(_collect_all_files_under(sub.id))
    except Exception:
        pass
    return results


class FileCommitService(CommonService):
    model = FileCommit

    @classmethod
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
        commit_id = get_uuid()
        now_ts = current_timestamp()
        now_dt = datetime_format(date_time=datetime.datetime.now())

        with DB.atomic():
            # 1. Get the latest (chain head) commit for this folder
            latest_commit = cls._get_latest_commit(folder_id)

            # 2. Begin creating the commit record
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

            # 3. Insert commit record
            FileCommit(**commit_data).save(force_insert=True)

            # 4. Build new tree state and process each file change
            tree_state = {}
            if latest_commit and latest_commit.tree_state:
                try:
                    tree_state = json.loads(latest_commit.tree_state)
                except (json.JSONDecodeError, TypeError):
                    tree_state = {}

            # 4a. Backfill parent_id for existing entries that lack it
            for fid, entry in tree_state.items():
                if isinstance(entry, dict) and "parent_id" not in entry:
                    pid = _get_file_parent_id(fid)
                    if pid:
                        entry["parent_id"] = pid

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
                    File.update({
                        "location": obj_key,
                        "size": len(content_bytes),
                        "update_time": current_timestamp(),
                    }).where(File.id == file_id).execute()

                    # Update tree state
                    file_parent = _get_file_parent_id(file_id)
                    tree_state[file_id] = {
                        "hash": content_hash,
                        "location": obj_key,
                        "name": change.get("file_name", ""),
                        "size": len(content_bytes),
                        "status": "1",
                        "parent_id": file_parent,
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
                    File.update({
                        "location": obj_key,
                        "size": len(content_bytes),
                        "update_time": current_timestamp(),
                    }).where(File.id == file_id).execute()

                    # Update tree state
                    file_parent = _get_file_parent_id(file_id)
                    tree_state[file_id] = {
                        "hash": content_hash,
                        "location": obj_key,
                        "name": change.get("file_name", tree_state.get(file_id, {}).get("name", "")),
                        "size": len(content_bytes),
                        "status": "1",
                        "parent_id": file_parent,
                    }

                elif op == "delete":
                    old_entry = tree_state.get(file_id, {})
                    old_hash = old_entry.get("hash", "")
                    old_location = old_entry.get("location", "")
                    if old_hash:
                        item["old_hash"] = old_hash
                        item["old_location"] = old_location

                    # Soft-delete the file record
                    File.update(status="0", update_time=current_timestamp()).where(
                        File.id == file_id
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
                    File.update(name=new_name, update_time=current_timestamp()).where(
                        File.id == file_id
                    ).execute()

                    # Update tree state
                    if file_id in tree_state:
                        tree_state[file_id]["name"] = new_name

                # Insert commit item
                FileCommitItem(**item).save(force_insert=True)

            # 5. Save the tree state snapshot
            tree_json = json.dumps(tree_state, ensure_ascii=False)
            cls.model.update(tree_state=tree_json).where(cls.model.id == commit_id).execute()

        _, commit = cls.get_by_id(commit_id)
        return commit

    @classmethod
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

        Compares tree_state snapshots (full file inventories), not commit
        items (which only capture per-commit deltas).  Falls back to
        FileCommitItem records for supplementary metadata (hash/location).

        Returns list of dicts with fields:
            file_id, file_name, operation, old_hash, new_hash, old_location, new_location
        """
        _, from_commit = cls.get_by_id(from_id)
        _, to_commit = cls.get_by_id(to_id)

        from_tree = {}
        to_tree = {}
        if from_commit and from_commit.tree_state:
            try:
                from_tree = json.loads(from_commit.tree_state)
            except Exception:
                pass
        if to_commit and to_commit.tree_state:
            try:
                to_tree = json.loads(to_commit.tree_state)
            except Exception:
                pass

        # Supplement with commit_item metadata for operations not captured
        # by tree_state alone (rename).
        from_items = {}
        try:
            for item in FileCommitItem.select().where(FileCommitItem.commit_id == from_id):
                from_items[item.file_id] = item
        except Exception:
            pass
        to_items = {}
        try:
            for item in FileCommitItem.select().where(FileCommitItem.commit_id == to_id):
                to_items[item.file_id] = item
        except Exception:
            pass

        all_file_ids = set(from_tree.keys()) | set(to_tree.keys())

        diff = []
        for fid in sorted(all_file_ids):
            from_entry = from_tree.get(fid)
            to_entry = to_tree.get(fid)

            from_item = from_items.get(fid)
            to_item = to_items.get(fid)

            from_hash = from_entry.get("hash", "") if isinstance(from_entry, dict) else ""
            to_hash = to_entry.get("hash", "") if isinstance(to_entry, dict) else ""
            from_status = from_entry.get("status", "1") if isinstance(from_entry, dict) else "1"
            to_status = to_entry.get("status", "1") if isinstance(to_entry, dict) else "1"
            from_name = from_entry.get("name", "") if isinstance(from_entry, dict) else ""
            to_name = to_entry.get("name", "") if isinstance(to_entry, dict) else ""

            if from_entry is not None and to_entry is None:
                # Present in from, absent in to → deleted
                diff.append({
                    "file_id": fid,
                    "file_name": from_name,
                    "operation": "delete",
                    "old_hash": from_hash or (from_item.new_hash if from_item else None),
                    "old_location": from_entry.get("location", "") if isinstance(from_entry, dict) else None,
                    "new_hash": None,
                    "new_location": None,
                })

            elif from_entry is None and to_entry is not None:
                # Present in to, absent in from → added
                diff.append({
                    "file_id": fid,
                    "file_name": to_name,
                    "operation": "add",
                    "old_hash": None,
                    "old_location": None,
                    "new_hash": to_hash or (to_item.new_hash if to_item else None),
                    "new_location": to_entry.get("location", "") if isinstance(to_entry, dict) else None,
                })

            else:
                # Both exist — check for changes
                changed = False
                operation = "modify"

                # Hash change
                if from_hash != to_hash:
                    changed = True

                # Status change (active ↔ deleted or vice versa in same entry)
                if from_status != to_status:
                    changed = True
                    operation = "delete" if to_status == "0" else "add"

                # Name change (rename)
                if from_name != to_name:
                    changed = True
                    operation = "rename"

                if changed:
                    old_loc = from_entry.get("location", "") if isinstance(from_entry, dict) else None
                    new_loc = to_entry.get("location", "") if isinstance(to_entry, dict) else None
                    diff.append({
                        "file_id": fid,
                        "file_name": to_name or from_name,
                        "operation": operation,
                        "old_hash": from_hash or (from_item.new_hash if from_item else None),
                        "old_location": old_loc or (from_item.new_location if from_item else None),
                        "new_hash": to_hash or (to_item.new_hash if to_item else None),
                        "new_location": new_loc or (to_item.new_location if to_item else None),
                    })

        return diff

    @classmethod
    @DB.connection_context()
    def get_uncommitted_changes(cls, folder_id):
        """Get uncommitted changes by comparing current File table with latest commit.

        Recursively scans all sub-folders under folder_id.
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

        # Get all current (live) files recursively under this folder
        current_files = _collect_all_files_under(folder_id)

        changes = []
        processed = set()

        # Check for modified and deleted files
        for fid, committed_entry in committed_files.items():
            processed.add(fid)
            if committed_entry.get("status") == "0":
                continue

            if fid in current_files:
                live_file = current_files[fid]
                live_hash = _compute_file_hash(folder_id, fid)
                committed_hash = committed_entry.get("hash", "")
                if live_hash and live_hash != committed_hash:
                    changes.append({
                        "file_id": fid,
                        "file_name": committed_entry.get("name", ""),
                        "operation": "modify",
                    })
            else:
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
        """Get the tree state snapshot for a commit as a hierarchical tree."""
        success, commit = cls.get_by_id(commit_id)
        if not success or not commit.tree_state:
            return {}
        try:
            tree_state = json.loads(commit.tree_state)
        except Exception:
            return {}
        return _build_hierarchical_tree(tree_state, commit.folder_id)

    @classmethod
    @DB.connection_context()
    def get_commit_file_content(cls, folder_id, commit_id, file_id):
        """Get file content as it existed in a given commit.

        Resolves the file's stored hash from the commit's tree_state first;
        if absent (file unchanged in this commit), walks the parent commit
        chain via parent_id until a FileCommitItem for the file is found.
        """
        success, commit = cls.get_by_id(commit_id)
        if not success:
            return None

        # 1. Try tree_state — the full snapshot at this commit
        if commit.tree_state:
            try:
                tree = json.loads(commit.tree_state)
                entry = tree.get(file_id)
                if isinstance(entry, dict):
                    h = entry.get("hash")
                    if h:
                        obj_path = f".objects/{h}"
                        storage_impl = settings.STORAGE_IMPL
                        if storage_impl:
                            return storage_impl.get(folder_id, obj_path)
            except Exception:
                pass

        # 2. Walk parent commits via parent_id until we find a
        #    FileCommitItem for this file_id.
        current_id = commit_id
        visited = set()
        while current_id and current_id not in visited:
            visited.add(current_id)
            item = FileCommitItem.select().where(
                FileCommitItem.commit_id == current_id,
                FileCommitItem.file_id == file_id,
            ).first()
            if item and item.new_hash:
                obj_path = f".objects/{item.new_hash}"
                storage_impl = settings.STORAGE_IMPL
                if storage_impl:
                    return storage_impl.get(folder_id, obj_path)
            # Move to parent
            parent_commit = cls.get_commit(current_id)
            if parent_commit and parent_commit.parent_id:
                current_id = parent_commit.parent_id
            else:
                break

        return None

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


def _lookup_folder_name(folder_id):
    """Look up a folder's display name from the File table."""
    try:
        row = File.get_or_none(File.id == folder_id)
        if row:
            return row.name
    except Exception:
        pass
    return folder_id


def _build_hierarchical_tree(tree_state, root_folder_id):
    """Build a recursive tree from a flat tree_state map.

    Returns {id, name, type: "folder", children: [{file|folder nodes}]}
    Sub-folder hierarchy is resolved from the File table's parent_id.
    """
    # Collect all unique folder IDs from parent_id fields
    folder_ids = {root_folder_id}
    for fid, entry in tree_state.items():
        if isinstance(entry, dict):
            pid = entry.get("parent_id") or root_folder_id
            folder_ids.add(pid)

    # Build a map of folder_id -> parent_folder_id from the File table
    folder_parent_map = {}
    for fid in folder_ids:
        if fid != root_folder_id:
            try:
                row = File.get_or_none(File.id == fid)
                if row:
                    folder_parent_map[fid] = row.parent_id
            except Exception:
                pass

    # Group file entries by parent_id
    files_by_parent = {}
    for fid, entry in tree_state.items():
        if not isinstance(entry, dict):
            continue
        pid = entry.get("parent_id") or root_folder_id
        files_by_parent.setdefault(pid, []).append((fid, entry))

    # Group sub-folder IDs by their parent folder
    children_by_folder = {}
    for sfid, ppid in folder_parent_map.items():
        children_by_folder.setdefault(ppid, []).append(sfid)

    def _build_node(node_id):
        node = {
            "id": node_id,
            "name": _lookup_folder_name(node_id),
            "type": "folder",
            "children": [],
        }
        # File children
        for fid, entry in files_by_parent.get(node_id, []):
            fn = {"id": fid, "name": entry.get("name", fid), "type": "file",
                  "hash": entry.get("hash", ""), "size": entry.get("size", 0),
                  "status": entry.get("status", "1")}
            if entry.get("location"):
                fn["location"] = entry["location"]
            node["children"].append(fn)
        # Sub-folder children (resolved from File table)
        for sfid in children_by_folder.get(node_id, []):
            child = _build_node(sfid)
            if child:
                node["children"].append(child)
        return node

    return _build_node(root_folder_id)


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
