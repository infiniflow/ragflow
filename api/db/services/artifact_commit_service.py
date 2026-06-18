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
"""Service for recording artifact-page edits as git-like commit rows.

A "commit" is created whenever the Artifact edit dialog Save fires and the
diff between the previous markdown and the new markdown is non-empty. No-op
saves are skipped at this layer.

Each row is self-contained (see ``ArtifactCommit.content_after``), so commit
history can be replayed without a parent-pointer chain.
"""

import difflib
import logging
from typing import Optional

from api.db.db_models import DB, ArtifactCommit, User
from api.db.services.common_service import CommonService
from common.misc_utils import get_uuid


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


class ArtifactCommitService(CommonService):
    model = ArtifactCommit

    @classmethod
    def record_edit(
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
        """Persist one edit as a commit row. Returns the new commit id, or
        ``None`` when the diff is empty (no-op save — skipped per the
        documented v1 contract).

        ``slug`` should be the full ``<page_type>/<name>`` form, matching
        the artifact_page row's ``slug_kwd``.
        """
        diff_text = _unified_diff(content_before or "", content_after or "", slug)
        if not diff_text:
            return None

        # Server-side title default keeps the dialog UX low-friction: callers
        # can pass title=None and we still get a meaningful audit entry.
        final_title = (title or "").strip() or f"Edit {slug}"
        commit_id = get_uuid()

        try:
            cls.insert(
                id=commit_id,
                tenant_id=tenant_id,
                kb_id=kb_id,
                page_type_kwd=page_type,
                slug=slug,
                user_id=user_id,
                title=final_title[:255],
                comments=comments or "",
                diff=diff_text,
                content_after=content_after or "",
            )
        except Exception:
            logging.exception(
                "ArtifactCommitService.record_edit: insert failed for kb=%s slug=%s",
                kb_id, slug,
            )
            return None

        return commit_id

    @classmethod
    @DB.connection_context()
    def list_for_page(
        cls,
        tenant_id: str,
        kb_id: str,
        slug: str,
        page: int = 1,
        page_size: int = 50,
    ) -> tuple[int, list[dict]]:
        """Return (total, items) for the History pane.

        Items are ordered newest-first via the composite index
        ``(tenant_id, kb_id, slug, create_time)``. Heavy columns (``diff``,
        ``content_after``) are intentionally excluded — the right-pane
        list view does not need them; clients fetch them lazily via
        ``get_detail`` when a row is expanded.

        ``user_nickname`` is resolved in one extra IN-query so the
        frontend doesn't N+1 it.
        """
        page = max(int(page or 1), 1)
        page_size = max(min(int(page_size or 50), 200), 1)

        base = cls.model.select(
            cls.model.id,
            cls.model.title,
            cls.model.comments,
            cls.model.user_id,
            cls.model.create_time,
            cls.model.create_date,
        ).where(
            (cls.model.tenant_id == tenant_id)
            & (cls.model.kb_id == kb_id)
            & (cls.model.slug == slug)
        )
        total = base.count()
        rows = list(
            base.order_by(cls.model.create_time.desc())
            .paginate(page, page_size)
            .dicts()
        )

        # Batch-resolve nicknames so the response is self-contained.
        user_ids = {r["user_id"] for r in rows if r.get("user_id")}
        nickname_by_id: dict[str, str] = {}
        if user_ids:
            try:
                for u in User.select(User.id, User.nickname).where(
                    User.id.in_(list(user_ids))
                ).dicts():
                    nickname_by_id[u["id"]] = u.get("nickname") or ""
            except Exception:
                logging.exception(
                    "ArtifactCommitService.list_for_page: nickname lookup failed",
                )

        for r in rows:
            r["user_nickname"] = nickname_by_id.get(r.get("user_id") or "", "")
        return total, rows

    @classmethod
    @DB.connection_context()
    def get_detail(
        cls, tenant_id: str, kb_id: str, commit_id: str,
    ) -> Optional[dict]:
        """Return one commit row including the heavy ``diff`` /
        ``content_after`` fields, or ``None`` when not found. Scoped by
        tenant + kb so a leaked commit id can't be read cross-tenant.
        """
        try:
            row = (
                cls.model.select()
                .where(
                    (cls.model.id == commit_id)
                    & (cls.model.tenant_id == tenant_id)
                    & (cls.model.kb_id == kb_id)
                )
                .dicts()
                .first()
            )
        except Exception:
            logging.exception(
                "ArtifactCommitService.get_detail: select failed for commit=%s",
                commit_id,
            )
            return None
        if not row:
            return None
        # Attach nickname for symmetry with list_for_page.
        nickname = ""
        if row.get("user_id"):
            try:
                u = User.get_or_none(User.id == row["user_id"])
                if u is not None:
                    nickname = u.nickname or ""
            except Exception:
                pass
        row["user_nickname"] = nickname
        return row
