import re
from werkzeug.security import check_password_hash
from api.db import ActiveEnum
from api.db.services import UserService
from api.db.joint_services.user_account_service import create_new_user
from api.db.services.canvas_service import UserCanvasService
from api.db.services.user_service import TenantService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils.crypt import decrypt
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
    def create_user(username, password, role="user") -> dict:
        # Validate the email address
        if not re.match(r"^[\w\._-]+@([\w_-]+\.)+[\w-]{2,}$", username):
            raise AdminException(f"Invalid email address: {username}!")
        # Check if the email address is already used
        if UserService.query(email=username):
            raise UserAlreadyExistsError(username)
        # Construct user info data
        user_info_dict = {
            "email": username,
            "nickname": "",  # ask user to edit it manually in settings.
            "password": decrypt(password),
            "login_channel": "password",
            "is_superuser": role == "admin",
        }
        return create_new_user(user_info_dict)

    @staticmethod
    def delete_user(username):
        # use email to delete
        raise AdminException("delete_user: not implemented")

    @staticmethod
    def update_user_password(username, new_password) -> str:
        # use email to find user. check exist and unique.
        user_list = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")
        # check new_password different from old.
        usr = user_list[0]
        psw = decrypt(new_password)
        if check_password_hash(usr.password, psw):
            return "Same password, no need to update!"
        # update password
        UserService.update_user_password(usr.id, psw)
        return "Password updated successfully!"

    @staticmethod
    def update_user_activate_status(username, activate_status: str):
        # use email to find user. check exist and unique.
        user_list = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")
        # check activate status different from new
        usr = user_list[0]
        # format activate_status before handle
        _activate_status = activate_status.lower()
        target_status = {
            'on': ActiveEnum.ACTIVE.value,
            'off': ActiveEnum.INACTIVE.value,
        }.get(_activate_status)
        if not target_status:
            raise AdminException(f"Invalid activate_status: {activate_status}")
        if target_status == usr.is_active:
            return f"User activate status is already {_activate_status}!"
        # update is_active
        UserService.update_user(usr.id, {"is_active": target_status})
        return f"Turn {_activate_status} user activate status successfully!"

class UserServiceMgr:

    @staticmethod
    def get_user_datasets(username):
        # use email to find user.
        user_list = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")
        # find tenants
        usr = user_list[0]
        tenants = TenantService.get_joined_tenants_by_user_id(usr.id)
        tenant_ids = [m["tenant_id"] for m in tenants]
        # filter permitted kb and owned kb
        return KnowledgebaseService.get_all_kb_by_tenant_ids(tenant_ids, usr.id)

    @staticmethod
    def get_user_agents(username):
        # use email to find user.
        user_list = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")
        # find tenants
        usr = user_list[0]
        tenants = TenantService.get_joined_tenants_by_user_id(usr.id)
        tenant_ids = [m["tenant_id"] for m in tenants]
        # filter permitted agents and owned agents
        return UserCanvasService.get_all_agents_by_tenant_ids(tenant_ids, usr.id)

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
