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
import difflib
import hashlib
import json
import logging
from typing import Optional

from api.db.db_models import DB, FileCommit, FileCommitItem, File, User
from api.db.services.common_service import CommonService
from api.db.services.file_service import FileService
from common import settings
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp, datetime_format

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------
# Artifact-commit extension
# ---------------------------------------------------------------------
# Artifact-page saves used to land in the retired ``ArtifactCommit`` table.
# They now flow through :class:`FileCommitService.record_page_edit`, which
# writes one FileCommit + one FileCommitItem per save with the artifact
# columns populated (title/comments on FileCommit; diff/content_after_*/
# slug_kwd/page_type_kwd on FileCommitItem).
#
# ``file_id`` for these commits is a stable content-hash of ``(kb_id, slug)``
# so per-page history queries can filter on it without a real File row —
# no pseudo-File / virtual-folder machinery is created, so the workspace
# UI stays free of ghost entries.
#
# ``folder_id`` is set to ``kb_id`` directly. The datasets URL prefix
# (``/datasets/<kb_id>/commits``) resolves the entity id to itself for
# this scope; workspace file-commit browsing still uses ``/folders/*`` or
# ``/workspace/*`` with the real folder id.
#
# Content storage for ``content_after`` is switched by a module-level
# constant so ops can move blobs between MinIO and the doc-store index
# without touching the schema.
ARTIFACT_CONTENT_STORAGE = "minio"  # one of {"minio", "es"}
_ARTIFACT_COMMIT_BUCKET_PREFIX = ".artifact_commits"
_ARTIFACT_ES_KWD = "artifact_commit_content"


def _artifact_file_id(kb_id: str, slug: str) -> str:
    """Deterministic 32-char id for the artifact-page 'file' identity.

    Not a real File row — just an index key that groups all commits for
    the same page. Hashed so slugs longer than 32 chars still fit.
    """
    return hashlib.md5(f"{kb_id}:{slug}".encode("utf-8")).hexdigest()


def _unified_diff(before: str, after: str, slug: str) -> str:
    """Return a unified diff between two markdown strings, or '' if equal."""
    if (before or "") == (after or ""):
        return ""
    return "".join(
        difflib.unified_diff(
            (before or "").splitlines(keepends=True),
            (after or "").splitlines(keepends=True),
            fromfile=f"a/{slug}",
            tofile=f"b/{slug}",
            n=3,
        )
    )


def _store_content_after(kb_id: str, content: str) -> tuple[str, str]:
    """Persist ``content`` per :data:`ARTIFACT_CONTENT_STORAGE`. Returns
    ``(storage_kind, location)`` for the row's persistence columns.

    Content-addressed by SHA-256 so re-saves with identical bodies share
    the same blob.
    """
    content_bytes = (content or "").encode("utf-8")
    content_hash = hashlib.sha256(content_bytes).hexdigest()

    if ARTIFACT_CONTENT_STORAGE == "minio":
        location = f"{_ARTIFACT_COMMIT_BUCKET_PREFIX}/{content_hash}"
        try:
            storage = settings.STORAGE_IMPL
            if storage is not None:
                storage.put(kb_id, location, content_bytes)
        except Exception:
            logging.exception(
                "record_page_edit: MinIO put failed for kb=%s hash=%s",
                kb_id,
                content_hash,
            )
        return "minio", location

    if ARTIFACT_CONTENT_STORAGE == "es":
        # Store as a single doc-store row so the same connector serves
        # reads. The row is not retrievable (available_int=0).
        from rag.nlp import search as _rag_search

        index = _rag_search.index_name(kb_id)  # kb-scoped index namespace
        payload = {
            "id": content_hash,
            "kb_id": kb_id,
            "doc_id": kb_id,
            "compile_kwd": _ARTIFACT_ES_KWD,
            "content_with_weight": content or "",
            "available_int": 0,
        }
        try:
            settings.docStoreConn.insert([payload], index, kb_id)
        except Exception:
            logging.exception(
                "record_page_edit: ES insert failed for kb=%s hash=%s",
                kb_id,
                content_hash,
            )
        return "es", content_hash

    # Unknown storage kind — fall through with empty location; the
    # detail path treats missing location as "content not recoverable".
    logging.warning(
        "record_page_edit: unknown ARTIFACT_CONTENT_STORAGE=%r; content not persisted",
        ARTIFACT_CONTENT_STORAGE,
    )
    return "", ""


def _read_content_after(kb_id: str, storage_kind: str, location: str) -> str:
    """Fetch the previously-stored artifact ``content_after`` blob.

    Returns ``""`` when the location is empty (workspace commits) or the
    blob is missing.
    """
    if not location:
        return ""
    try:
        if storage_kind == "minio":
            storage = settings.STORAGE_IMPL
            if storage is None:
                return ""
            raw = storage.get(kb_id, location)
            if isinstance(raw, (bytes, bytearray)):
                return raw.decode("utf-8", errors="replace")
            return str(raw or "")
        if storage_kind == "es":
            from rag.nlp import search as _rag_search

            index = _rag_search.index_name(kb_id)
            row = settings.docStoreConn.get(location, index, [kb_id])
            if isinstance(row, dict):
                return row.get("content_with_weight") or ""
            return ""
    except Exception:
        logging.exception(
            "get_page_commit: content read failed kb=%s storage=%s loc=%s",
            kb_id,
            storage_kind,
            location,
        )
    return ""


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
                    File.update(
                        {
                            "location": obj_key,
                            "size": len(content_bytes),
                            "update_time": current_timestamp(),
                        }
                    ).where(File.id == file_id).execute()

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
                    File.update(
                        {
                            "location": obj_key,
                            "size": len(content_bytes),
                            "update_time": current_timestamp(),
                        }
                    ).where(File.id == file_id).execute()

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
                    File.update(status="0", update_time=current_timestamp()).where(File.id == file_id).execute()

                    # Remove from tree state (mark deleted)
                    if file_id in tree_state:
                        tree_state[file_id]["status"] = "0"

                elif op == "rename":
                    old_name = change.get("old_name", "")
                    new_name = change.get("new_name", "")
                    item["old_name"] = old_name
                    item["new_name"] = new_name

                    # Update the file record name
                    File.update(name=new_name, update_time=current_timestamp()).where(File.id == file_id).execute()

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
            return cls.model.select().where(cls.model.folder_id == folder_id).order_by(cls.model.create_time.desc()).first()
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
                diff.append(
                    {
                        "file_id": fid,
                        "file_name": from_name,
                        "operation": "delete",
                        "old_hash": from_hash or (from_item.new_hash if from_item else None),
                        "old_location": from_entry.get("location", "") if isinstance(from_entry, dict) else None,
                        "new_hash": None,
                        "new_location": None,
                    }
                )

            elif from_entry is None and to_entry is not None:
                # Present in to, absent in from → added
                diff.append(
                    {
                        "file_id": fid,
                        "file_name": to_name,
                        "operation": "add",
                        "old_hash": None,
                        "old_location": None,
                        "new_hash": to_hash or (to_item.new_hash if to_item else None),
                        "new_location": to_entry.get("location", "") if isinstance(to_entry, dict) else None,
                    }
                )

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
                    diff.append(
                        {
                            "file_id": fid,
                            "file_name": to_name or from_name,
                            "operation": operation,
                            "old_hash": from_hash or (from_item.new_hash if from_item else None),
                            "old_location": old_loc or (from_item.new_location if from_item else None),
                            "new_hash": to_hash or (to_item.new_hash if to_item else None),
                            "new_location": new_loc or (to_item.new_location if to_item else None),
                        }
                    )

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
                    changes.append(
                        {
                            "file_id": fid,
                            "file_name": committed_entry.get("name", ""),
                            "operation": "modify",
                        }
                    )
            else:
                if FileService.get_or_none(id=fid) is None:
                    changes.append(
                        {
                            "file_id": fid,
                            "file_name": committed_entry.get("name", ""),
                            "operation": "delete",
                        }
                    )

        # Check for newly added files
        for fid, live_file in current_files.items():
            if fid not in processed:
                changes.append(
                    {
                        "file_id": fid,
                        "file_name": live_file.name,
                        "operation": "add",
                    }
                )

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
            item = (
                FileCommitItem.select()
                .where(
                    FileCommitItem.commit_id == current_id,
                    FileCommitItem.file_id == file_id,
                )
                .first()
            )
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

    # ------------------------------------------------------------------
    # Artifact-page commit surface
    # ------------------------------------------------------------------

    @classmethod
    def record_page_edit(
        cls,
        *,
        tenant_id: str,
        kb_id: str,
        page_type: str,
        slug: str,
        content_before: str,
        content_after: str,
        title: Optional[str] = None,
        comments: Optional[str] = None,
        user_id: Optional[str] = None,
    ) -> Optional[str]:
        """Persist one artifact-page edit as a FileCommit + FileCommitItem.

        Returns the new commit id, or ``None`` when the diff is empty
        (no-op save — skipped per the documented v1 contract).

        Bypasses :func:`create_commit` because artifact commits have no
        real ``File`` row backing them and don't participate in the
        workspace ``tree_state`` snapshot chain.
        """
        diff_text = _unified_diff(content_before or "", content_after or "", slug)
        if not diff_text:
            return None

        title_ts = datetime.datetime.now().strftime("%Y%m%d%H%M%S")
        final_title = f"{(title or '').strip() or f'{title_ts} {slug}'} "
        commit_id = get_uuid()
        item_id = get_uuid()
        file_id = _artifact_file_id(kb_id, slug)
        now_ts = current_timestamp()
        now_dt = datetime_format(date_time=datetime.datetime.now())

        # Persist the post-save markdown per the configured storage.
        # A failure here logs but doesn't block the commit row — the diff
        # is still meaningful without content_after.
        storage_kind, location = _store_content_after(kb_id, content_after or "")

        # Chain to the previous commit for this page so the history stays
        # ordered even under concurrent writes (auto-regen + user edit).
        parent = (
            FileCommit.select(FileCommit.id)
            .join(
                FileCommitItem,
                on=(FileCommitItem.commit_id == FileCommit.id),
            )
            .where((FileCommit.folder_id == kb_id) & (FileCommitItem.file_id == file_id))
            .order_by(FileCommit.create_time.desc())
            .first()
        )
        parent_id = parent.id if parent else None

        try:
            with DB.atomic():
                FileCommit(
                    id=commit_id,
                    folder_id=kb_id,
                    parent_id=parent_id,
                    # ``message`` stays populated with the same string as
                    # ``title`` so any generic file-commit consumer still
                    # renders something sensible.
                    message=final_title[:512],
                    author_id=user_id or "",
                    file_count=1,
                    tree_state=None,
                    title=final_title[:255],
                    comments=comments or "",
                    create_time=now_ts,
                    create_date=now_dt,
                    update_time=now_ts,
                    update_date=now_dt,
                ).save(force_insert=True)

                FileCommitItem(
                    id=item_id,
                    commit_id=commit_id,
                    file_id=file_id,
                    operation="modify" if content_before else "add",
                    diff=diff_text,
                    content_after_storage=storage_kind or None,
                    content_after_location=location or None,
                    slug_kwd=slug,
                    page_type_kwd=page_type,
                    create_time=now_ts,
                    create_date=now_dt,
                    update_time=now_ts,
                    update_date=now_dt,
                ).save(force_insert=True)
        except Exception:
            logging.exception(
                "record_page_edit: insert failed for kb=%s slug=%s",
                kb_id,
                slug,
            )
            return None

        return commit_id

    @classmethod
    @DB.connection_context()
    def list_page_commits(
        cls,
        tenant_id: str,
        kb_id: str,
        slug: str,
        page: int = 1,
        page_size: int = 50,
    ) -> tuple[int, list[dict]]:
        """Return (total, items) for one artifact page's history.

        Filters by ``FileCommitItem.slug_kwd``; joins User for nickname.
        Heavy columns (``diff``, ``content_after``) are excluded — the
        detail path fetches them lazily.
        """
        page = max(int(page or 1), 1)
        page_size = max(min(int(page_size or 50), 200), 1)
        file_id = _artifact_file_id(kb_id, slug)

        base = (
            FileCommit.select(
                FileCommit.id,
                FileCommit.title,
                FileCommit.comments,
                FileCommit.author_id,
                FileCommit.create_time,
                FileCommit.create_date,
            )
            .join(FileCommitItem, on=(FileCommitItem.commit_id == FileCommit.id))
            .where((FileCommit.folder_id == kb_id) & (FileCommitItem.file_id == file_id) & (FileCommitItem.slug_kwd == slug))
        )
        total = base.count()
        rows = list(base.order_by(FileCommit.create_time.desc()).paginate(page, page_size).dicts())
        # Preserve the previous response key so callers only re-key once.
        for r in rows:
            r["user_id"] = r.pop("author_id", None)

        user_ids = {r["user_id"] for r in rows if r.get("user_id")}
        nickname_by_id: dict[str, str] = {}
        if user_ids:
            try:
                for u in User.select(User.id, User.nickname).where(User.id.in_(list(user_ids))).dicts():
                    nickname_by_id[u["id"]] = u.get("nickname") or ""
            except Exception:
                logging.exception(
                    "list_page_commits: nickname lookup failed",
                )
        for r in rows:
            r["user_nickname"] = nickname_by_id.get(r.get("user_id") or "", "")
        return total, rows

    @classmethod
    @DB.connection_context()
    def get_page_commit_detail(
        cls,
        tenant_id: str,
        kb_id: str,
        commit_id: str,
    ) -> Optional[dict]:
        """Return one artifact commit including ``diff`` +
        ``content_after`` (resolved from storage), or ``None`` when not
        found. Scoped by ``folder_id == kb_id`` so a leaked commit id
        can't be read cross-tenant.
        """
        commit = FileCommit.get_or_none(
            (FileCommit.id == commit_id) & (FileCommit.folder_id == kb_id),
        )
        if commit is None:
            return None
        item = FileCommitItem.get_or_none(FileCommitItem.commit_id == commit_id)
        if item is None:
            return None

        content_after = _read_content_after(
            kb_id,
            item.content_after_storage or "",
            item.content_after_location or "",
        )

        nickname = ""
        if commit.author_id:
            try:
                u = User.get_or_none(User.id == commit.author_id)
                if u is not None:
                    nickname = u.nickname or ""
            except Exception:
                pass

        return {
            "id": commit.id,
            "tenant_id": tenant_id,
            "kb_id": kb_id,
            "page_type_kwd": item.page_type_kwd,
            "slug": item.slug_kwd,
            "user_id": commit.author_id or None,
            "user_nickname": nickname,
            "title": commit.title,
            "comments": commit.comments,
            "diff": item.diff,
            "content_after": content_after,
            "create_time": commit.create_time,
            "create_date": commit.create_date,
        }

    @classmethod
    @DB.connection_context()
    def get_file_version_history(cls, file_id):
        """Get version history for a specific file across all commits.

        Returns list of dicts: [{"commit_id", "operation", "hash", "create_time", "message"}]
        """
        items = FileCommitItem.select().where(FileCommitItem.file_id == file_id).order_by(FileCommitItem.create_time.desc())

        versions = []
        for item in items:
            commit = cls.get_commit(item.commit_id)
            if commit:
                versions.append(
                    {
                        "commit_id": item.commit_id,
                        "operation": item.operation,
                        "hash": item.new_hash or item.old_hash or "",
                        "create_time": item.create_time,
                        "message": commit.message,
                    }
                )

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
            fn = {"id": fid, "name": entry.get("name", fid), "type": "file", "hash": entry.get("hash", ""), "size": entry.get("size", 0), "status": entry.get("status", "1")}
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
