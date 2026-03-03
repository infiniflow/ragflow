import json
import logging
import time

from api.db.db_models import UserCanvasVersion, DB
from api.db.services.common_service import CommonService
from peewee import DoesNotExist


class UserCanvasVersionService(CommonService):
    model = UserCanvasVersion

    @staticmethod
    def build_version_title(user_nickname, agent_title, ts=None):
        tenant = str(user_nickname or "").strip() or "tenant"
        title = str(agent_title or "").strip() or "agent"
        stamp = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(ts)) if ts is not None else time.strftime("%Y-%m-%d %H:%M:%S")
        return "{0}_{1}_{2}".format(tenant, title, stamp)

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
            return json.loads(json.dumps(normalized, ensure_ascii=False))
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
                  cls.model.update_time]
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
            user_canvas_version = cls.model.select().where(cls.model.user_canvas_id == user_canvas_id).order_by(
                cls.model.create_time.desc())
            if user_canvas_version.count() > 20:
                delete_ids = []
                for i in range(20, user_canvas_version.count()):
                    delete_ids.append(user_canvas_version[i].id)

                cls.delete_by_ids(delete_ids)
            return True
        except DoesNotExist:
            return None
        except Exception:
            return None

    @classmethod
    @DB.connection_context()
    def save_or_replace_latest(cls, user_canvas_id, dsl, title=None, description=None):
        """
        Persist a canvas snapshot into version history.

        If the latest version has the same DSL content, update that version in place
        instead of creating a new row.
        """
        try:
            normalized_dsl = cls._normalize_dsl(dsl)
            latest = (
                cls.model.select()
                .where(cls.model.user_canvas_id == user_canvas_id)
                .order_by(cls.model.create_time.desc())
                .first()
            )

            if latest and cls._normalize_dsl(latest.dsl) == normalized_dsl:
                update_data = {"dsl": normalized_dsl}
                if title is not None:
                    update_data["title"] = title
                if description is not None:
                    update_data["description"] = description
                cls.update_by_id(latest.id, update_data)
                cls.delete_all_versions(user_canvas_id)
                return latest.id, False

            insert_data = {"user_canvas_id": user_canvas_id, "dsl": normalized_dsl}
            if title is not None:
                insert_data["title"] = title
            if description is not None:
                insert_data["description"] = description
            cls.insert(**insert_data)
            cls.delete_all_versions(user_canvas_id)
            return None, True
        except Exception as e:
            logging.exception(e)
            return None, None
