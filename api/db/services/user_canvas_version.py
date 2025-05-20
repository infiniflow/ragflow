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
    def delete_all_versions(cls, user_canvas_id):
        try:
            user_canvas_version = cls.model.select().where(cls.model.user_canvas_id == user_canvas_id).order_by(cls.model.create_time.desc())
            if user_canvas_version.count() > 20:
                for i in range(20, user_canvas_version.count()):
                    cls.delete(user_canvas_version[i].id)
            return True
        except DoesNotExist:
            return None
        except Exception:
            return None



