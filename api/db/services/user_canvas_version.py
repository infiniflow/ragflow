from api.db.db_models import UserCanvasVersion, DB
from api.db.services.common_service import CommonService
from peewee import DoesNotExist


class UserCanvasVersionService(CommonService):
    model = UserCanvasVersion

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
