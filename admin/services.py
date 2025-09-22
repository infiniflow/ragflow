from api.db.services import UserService
from exceptions import AdminException
from config import SERVICE_CONFIGS

class UserMgr:
    @staticmethod
    def get_all_users():
        users = UserService.get_all_users()
        result = []
        for user in users:
            result.append({'email': user.email, 'nickname': user.nickname, 'create_date': user.create_date, 'is_active': user.is_active})
        return result

    @staticmethod
    def get_user_details(username):
        raise AdminException("get_user_details: not implemented")

    @staticmethod
    def create_user(username, password, role="user"):
        raise AdminException("create_user: not implemented")

    @staticmethod
    def delete_user(username):
        raise AdminException("delete_user: not implemented")

    @staticmethod
    def update_user_password(username, new_password):
        raise AdminException("update_user_password: not implemented")

class ServiceMgr:

    @staticmethod
    def get_all_services():
        result = []
        configs = SERVICE_CONFIGS.configs
        for config in configs:
            result.append(config.to_dict())
        return result

    @staticmethod
    def get_services_by_type(service_type_str: str):
        raise AdminException("get_services_by_type: not implemented")

    @staticmethod
    def get_service_details(service_id: int):
        raise AdminException("get_service_details: not implemented")

    @staticmethod
    def shutdown_service(service_id: int):
        raise AdminException("shutdown_service: not implemented")

    @staticmethod
    def restart_service(service_id: int):
        raise AdminException("restart_service: not implemented")
