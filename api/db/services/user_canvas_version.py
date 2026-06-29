import hashlib
import json
import logging
import time

from agent.dsl_migration import normalize_chunker_dsl
from api.db.db_models import CanvasBranch, UserCanvasVersion, DB
from api.db.services.common_service import CommonService
from common.misc_utils import get_uuid
from peewee import DoesNotExist


class UserCanvasVersionService(CommonService):
    model = UserCanvasVersion

    # Build a stable display name for saved snapshots.
    @staticmethod
    def build_version_title(user_nickname, agent_title, ts=None):
        tenant = str(user_nickname or "").strip() or "tenant"
        title = str(agent_title or "").strip() or "agent"
        stamp = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(ts)) if ts is not None else time.strftime("%Y-%m-%d %H:%M:%S")
        return "{0}_{1}_{2}".format(tenant, title, stamp)

    # Normalize DSL before comparing or writing version content.
    @staticmethod
    def _normalize_dsl(dsl):
        normalized = dsl
        if isinstance(normalized, str):
            try:
                normalized = json.loads(normalized)
            except Exception as e:
                raise ValueError("Invalid DSL JSON string.") from e

        if not isinstance(normalized, dict):
            raise ValueError("DSL must be a JSON object.")

        try:
            return json.loads(json.dumps(normalize_chunker_dsl(normalized), ensure_ascii=False))
        except Exception as e:
            raise ValueError("DSL is not JSON-serializable.") from e

    @classmethod
    @DB.connection_context()
    def list_by_canvas_id(cls, user_canvas_id):
        try:
            user_canvas_version = cls.model.select(
                *[cls.model.id,
                  cls.model.create_time,
                  cls.model.title,
                  cls.model.create_date,
                  cls.model.update_date,
                  cls.model.user_canvas_id,
                  cls.model.update_time,
                  cls.model.release]
            ).where(cls.model.user_canvas_id == user_canvas_id)
            return user_canvas_version
        except DoesNotExist:
            return None
        except Exception:
            return None

    @classmethod
    @DB.connection_context()
    def get_all_canvas_version_by_canvas_ids(cls, canvas_ids):
        fields = [cls.model.id]
        versions = cls.model.select(*fields).where(cls.model.user_canvas_id.in_(canvas_ids))
        versions.order_by(cls.model.create_time.asc())
        offset, limit = 0, 100
        res = []
        while True:
            version_batch = versions.offset(offset).limit(limit)
            _temp = list(version_batch.dicts())
            if not _temp:
                break
            res.extend(_temp)
            offset += limit
        return res

    @classmethod
    @DB.connection_context()
    def delete_all_versions(cls, user_canvas_id):
        try:
            # Only get unpublished versions (False or None), keep all released versions
            unpublished = cls.model.select().where(cls.model.user_canvas_id == user_canvas_id, (~cls.model.release) | (cls.model.release.is_null(True))).order_by(cls.model.create_time.desc())

            # Only delete old unpublished versions beyond the limit
            if unpublished.count() > 20:
                delete_ids = [v.id for v in unpublished[20:]]
                cls.delete_by_ids(delete_ids)

            return True
        except DoesNotExist:
            return None
        except Exception:
            return None

    @classmethod
    @DB.connection_context()
    def _get_latest_by_canvas_id(cls, user_canvas_id, only_released=False):
        """Get the latest version for a canvas, optionally filtered by release status."""
        try:
            query = cls.model.select().where(cls.model.user_canvas_id == user_canvas_id)
            if only_released:
                query = query.where(cls.model.release)
            return query.order_by(cls.model.create_time.desc()).first()
        except DoesNotExist:
            return None
        except Exception as e:
            logging.exception(e)
            return None

    @classmethod
    def get_latest_released(cls, user_canvas_id):
        """Get the latest released version for a canvas."""
        return cls._get_latest_by_canvas_id(user_canvas_id, only_released=True)

    @classmethod
    def get_latest_version_title(cls, user_canvas_id, release_mode=False):
        """Get the version title for a canvas based on release_mode.

        Args:
            user_canvas_id: The canvas ID
            release_mode: If True, get the latest released version title;
                         If False, get the latest version title (regardless of release status)
        """
        latest = cls._get_latest_by_canvas_id(user_canvas_id, only_released=release_mode)
        return latest.title if latest else None

    @classmethod
    @DB.connection_context()
    def save_or_replace_latest(cls, user_canvas_id, dsl, title=None, description=None, release=None):
        """
        Persist a canvas snapshot into version history.

        If the latest version has the same DSL content, update that version in place
        instead of creating a new row.

        Exception: If the latest version is released (release=True) and current save is not,
        create a new version to protect the released version.
        """
        try:
            normalized_dsl = cls._normalize_dsl(dsl)
            latest = (
                cls.model.select()
                .where(cls.model.user_canvas_id == user_canvas_id)
                .order_by(cls.model.create_time.desc())
                .first()
            )

            # Repeated saves with the same DSL only refresh the latest snapshot.
            if latest and cls._normalize_dsl(latest.dsl) == normalized_dsl:
                # Protect released version: if latest is released and current is not,
                # create a new version instead of updating
                if latest.release and not release:
                    insert_data = {"user_canvas_id": user_canvas_id, "dsl": normalized_dsl}
                    if title is not None:
                        insert_data["title"] = title
                    if description is not None:
                        insert_data["description"] = description
                    if release is not None:
                        insert_data["release"] = release
                    cls.insert(**insert_data)
                    cls.delete_all_versions(user_canvas_id)
                    return None, True

                # Normal case: update existing version
                # DSL unchanged: do NOT update title to preserve version identity
                # Only update dsl (for normalization consistency), description, and release
                update_data = {"dsl": normalized_dsl}
                if description is not None:
                    update_data["description"] = description
                if release is not None:
                    update_data["release"] = release
                cls.update_by_id(latest.id, update_data)
                cls.delete_all_versions(user_canvas_id)
                return latest.id, False

            # Real content changes create a new snapshot.
            insert_data = {"user_canvas_id": user_canvas_id, "dsl": normalized_dsl}
            if title is not None:
                insert_data["title"] = title
            if description is not None:
                insert_data["description"] = description
            if release is not None:
                insert_data["release"] = release
            cls.insert(**insert_data)
            cls.delete_all_versions(user_canvas_id)
            return None, True
        except Exception as e:
            logging.exception(e)
            return None, None


class CanvasBranchService(CommonService):
    model = CanvasBranch

    @classmethod
    @DB.connection_context()
    def create_branch(cls, canvas_id: str, branch_name: str, dsl_snapshot: dict, traffic_weight: int = 0) -> dict:
        """Snapshot the current live DSL into a named branch."""
        branch_id = get_uuid()
        cls.model.insert({
            "id": branch_id,
            "canvas_id": canvas_id,
            "branch_name": branch_name,
            "dsl_snapshot": dsl_snapshot,
            "traffic_weight": max(0, min(100, traffic_weight)),
            "is_active": True,
        }).execute()
        e, branch = cls.get_by_id(branch_id)
        return branch.to_dict() if e else {}

    @classmethod
    @DB.connection_context()
    def get_active_branches(cls, canvas_id: str) -> list:
        """Return all active branches for a canvas."""
        return list(
            cls.model.select()
            .where(cls.model.canvas_id == canvas_id, cls.model.is_active == True)  # noqa: E712
            .order_by(cls.model.create_time.asc())
            .dicts()
        )

    @classmethod
    @DB.connection_context()
    def set_traffic_split(cls, canvas_id: str, branch_id: str, weight: int) -> bool:
        """Update a branch's traffic weight (0-100). No server restart required."""
        weight = max(0, min(100, weight))
        rows = (
            cls.model.update(traffic_weight=weight)
            .where(cls.model.id == branch_id, cls.model.canvas_id == canvas_id)
            .execute()
        )
        return rows > 0

    @classmethod
    @DB.connection_context()
    def promote_branch(cls, canvas_id: str, branch_id: str) -> dict:
        """
        Promote branch_id to live DSL on the parent UserCanvas and zero all
        traffic weights (atomic within the DB connection context).
        Returns the promoted branch dict or empty dict on failure.
        """
        from api.db.db_models import UserCanvas

        e, branch = cls.get_by_id(branch_id)
        if not e or str(branch.canvas_id) != str(canvas_id):
            return {}

        with DB.atomic():
            UserCanvas.update(dsl=branch.dsl_snapshot).where(UserCanvas.id == canvas_id).execute()
            cls.model.update(traffic_weight=0).where(cls.model.canvas_id == canvas_id).execute()

        return branch.to_dict()

    @classmethod
    @DB.connection_context()
    def rollback_branch(cls, canvas_id: str, branch_id: str) -> bool:
        """
        Restore canvas DSL from branch_id, clear all traffic weights, and
        deactivate the rolled-back branch.
        """
        from api.db.db_models import UserCanvas

        e, branch = cls.get_by_id(branch_id)
        if not e or str(branch.canvas_id) != str(canvas_id):
            return False

        with DB.atomic():
            UserCanvas.update(dsl=branch.dsl_snapshot).where(UserCanvas.id == canvas_id).execute()
            cls.model.update(traffic_weight=0).where(cls.model.canvas_id == canvas_id).execute()
            cls.model.update(is_active=False).where(cls.model.id == branch_id).execute()

        return True

    @staticmethod
    def has_active_branches(canvas_id: str) -> bool:
        """Return True if the canvas has at least one active branch with weight > 0."""
        with DB.connection_context():
            return (
                CanvasBranch.select()
                .where(
                    CanvasBranch.canvas_id == canvas_id,
                    CanvasBranch.is_active == True,  # noqa: E712
                    CanvasBranch.traffic_weight > 0,
                )
                .exists()
            )

    @staticmethod
    def resolve_branch_for_session(session_id: str, canvas_id: str) -> "CanvasBranch | None":
        """
        Deterministic, sticky branch selection based on session_id hash.

        Branches are sorted by create_time ascending and treated as buckets
        sized by their traffic_weight on a 0-100 scale. A hash of the
        session_id maps the session to exactly one bucket; any remainder
        below 100 routes to the live canvas DSL (control group). If no
        active branch has weight > 0 the function returns None.
        """
        with DB.connection_context():
            branches = list(
                CanvasBranch.select()
                .where(
                    CanvasBranch.canvas_id == canvas_id,
                    CanvasBranch.is_active == True,  # noqa: E712
                    CanvasBranch.traffic_weight > 0,
                )
                .order_by(CanvasBranch.create_time.asc())
            )

        if not branches:
            return None

        total = sum(b.traffic_weight for b in branches)
        if total <= 0:
            return None

        digest = int(hashlib.sha256(session_id.encode()).hexdigest(), 16)
        if total < 100:
            slot = digest % 100
            if slot >= total:
                return None
        else:
            slot = digest % total

        cumulative = 0
        for branch in branches:
            cumulative += branch.traffic_weight
            if slot < cumulative:
                return branch

        return branches[-1]
