import re
from api.db.services import UserService
from api.db.joint_services.user_account_service import create_new_user
from api.utils import decrypt, get_format_time
from exceptions import AdminException, UserAlreadyExistsError, UserNotFoundError
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
        # use email to query
        users = UserService.query_user_by_email(username)
        result = []
        for user in users:
            result.append({
                'email': user.email,
                'language': user.language,
                'last_login_time': user.last_login_time,
                'is_authenticated': user.is_authenticated,
                'is_active': user.is_active,
                'is_anonymous': user.is_anonymous,
                'login_channel': user.login_channel,
                'status': user.status,
                'is_superuser': user.is_superuser,
                'create_date': user.create_date,
                'update_date': user.update_date
            })
        return result

    @staticmethod
    def create_user(username, password, role="user"):
        # Validate the email address
        if not re.match(r"^[\w\._-]+@([\w_-]+\.)+[\w-]{2,}$", username):
            raise AdminException(f"Invalid email address: {username}!")
        # Check if the email address is already used
        if UserService.query(email=username):
            raise UserAlreadyExistsError(f"User {username} already exists!")
        # Construct user info data
        user_info_dict = {
            "email": username,
            "nickname": "",  # ask user to edit it manually in settings.
            "password": decrypt(password),
            "login_channel": "password",
            "last_login_time": get_format_time(),
            "is_superuser": role == "admin",
        }
        return create_new_user(user_info_dict)

    @staticmethod
    def delete_user(username):
        # use email to delete
        raise AdminException("delete_user: not implemented")

    @staticmethod
    def update_user_password(username, new_password):
        # use email to find user
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
