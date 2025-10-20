#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

import secrets

from flask import Blueprint, request
from flask_login import current_user, logout_user, login_required

from auth import login_verify, login_admin, check_admin_auth
from responses import success_response, error_response
from services import UserMgr, ServiceMgr, UserServiceMgr
from roles import RoleMgr
from api.common.exceptions import AdminException

admin_bp = Blueprint('admin', __name__, url_prefix='/api/v1/admin')


@admin_bp.route('/login', methods=['POST'])
def login():
    if not request.json:
        return error_response('Authorize admin failed.' ,400)
    email = request.json.get("email", "")
    password = request.json.get("password", "")
    return login_admin(email, password)


@admin_bp.route('/logout', methods=['GET'])
@login_required
def logout():
    current_user.access_token = f"INVALID_{secrets.token_hex(16)}"
    current_user.save()
    logout_user()
    return success_response(True)


@admin_bp.route('/auth', methods=['GET'])
@login_verify
def auth_admin():
    try:
        return success_response(None, "Admin is authorized", 0)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users', methods=['GET'])
@login_required
@check_admin_auth
def list_users():
    try:
        users = UserMgr.get_all_users()
        return success_response(users, "Get all users", 0)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users', methods=['POST'])
@login_required
@check_admin_auth
def create_user():
    try:
        data = request.get_json()
        if not data or 'username' not in data or 'password' not in data:
            return error_response("Username and password are required", 400)

        username = data['username']
        password = data['password']
        role = data.get('role', 'user')

        res = UserMgr.create_user(username, password, role)
        if res["success"]:
            user_info = res["user_info"]
            user_info.pop("password")  # do not return password
            return success_response(user_info, "User created successfully")
        else:
            return error_response("create user failed")

    except AdminException as e:
        return error_response(e.message, e.code)
    except Exception as e:
        return error_response(str(e))


@admin_bp.route('/users/<username>', methods=['DELETE'])
@login_required
@check_admin_auth
def delete_user(username):
    try:
        res = UserMgr.delete_user(username)
        if res["success"]:
            return success_response(None, res["message"])
        else:
            return error_response(res["message"])

    except AdminException as e:
        return error_response(e.message, e.code)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users/<username>/password', methods=['PUT'])
@login_required
@check_admin_auth
def change_password(username):
    try:
        data = request.get_json()
        if not data or 'new_password' not in data:
            return error_response("New password is required", 400)

        new_password = data['new_password']
        msg = UserMgr.update_user_password(username, new_password)
        return success_response(None, msg)

    except AdminException as e:
        return error_response(e.message, e.code)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users/<username>/activate', methods=['PUT'])
@login_required
@check_admin_auth
def alter_user_activate_status(username):
    try:
        data = request.get_json()
        if not data or 'activate_status' not in data:
            return error_response("Activation status is required", 400)
        activate_status = data['activate_status']
        msg = UserMgr.update_user_activate_status(username, activate_status)
        return success_response(None, msg)
    except AdminException as e:
        return error_response(e.message, e.code)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users/<username>', methods=['GET'])
@login_required
@check_admin_auth
def get_user_details(username):
    try:
        user_details = UserMgr.get_user_details(username)
        return success_response(user_details)

    except AdminException as e:
        return error_response(e.message, e.code)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users/<username>/datasets', methods=['GET'])
@login_required
@check_admin_auth
def get_user_datasets(username):
    try:
        datasets_list = UserServiceMgr.get_user_datasets(username)
        return success_response(datasets_list)

    except AdminException as e:
        return error_response(e.message, e.code)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users/<username>/agents', methods=['GET'])
@login_required
@check_admin_auth
def get_user_agents(username):
    try:
        agents_list = UserServiceMgr.get_user_agents(username)
        return success_response(agents_list)

    except AdminException as e:
        return error_response(e.message, e.code)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/services', methods=['GET'])
@login_required
@check_admin_auth
def get_services():
    try:
        services = ServiceMgr.get_all_services()
        return success_response(services, "Get all services", 0)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/service_types/<service_type>', methods=['GET'])
@login_required
@check_admin_auth
def get_services_by_type(service_type_str):
    try:
        services = ServiceMgr.get_services_by_type(service_type_str)
        return success_response(services)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/services/<service_id>', methods=['GET'])
@login_required
@check_admin_auth
def get_service(service_id):
    try:
        services = ServiceMgr.get_service_details(service_id)
        return success_response(services)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/services/<service_id>', methods=['DELETE'])
@login_required
@check_admin_auth
def shutdown_service(service_id):
    try:
        services = ServiceMgr.shutdown_service(service_id)
        return success_response(services)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/services/<service_id>', methods=['PUT'])
@login_required
@check_admin_auth
def restart_service(service_id):
    try:
        services = ServiceMgr.restart_service(service_id)
        return success_response(services)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/roles', methods=['POST'])
@login_required
@check_admin_auth
def create_role():
    try:
        data = request.get_json()
        if not data or 'role_name' not in data:
            return error_response("Role name is required", 400)
        role_name: str = data['role_name']
        description: str = data['description']
        res = RoleMgr.create_role(role_name, description)
        return success_response(res)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/roles/<role_name>', methods=['PUT'])
@login_required
@check_admin_auth
def update_role(role_name: str):
    try:
        data = request.get_json()
        if not data or 'description' not in data:
            return error_response("Role description is required", 400)
        description: str = data['description']
        res = RoleMgr.update_role_description(role_name, description)
        return success_response(res)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/roles/<role_name>', methods=['DELETE'])
@login_required
@check_admin_auth
def delete_role(role_name: str):
    try:
        res = RoleMgr.delete_role(role_name)
        return success_response(res)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/roles', methods=['GET'])
@login_required
@check_admin_auth
def list_roles():
    try:
        res = RoleMgr.list_roles()
        return success_response(res)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/roles/<role_name>/permission', methods=['GET'])
@login_required
@check_admin_auth
def get_role_permission(role_name: str):
    try:
        res = RoleMgr.get_role_permission(role_name)
        return success_response(res)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/roles/<role_name>/permission', methods=['POST'])
@login_required
@check_admin_auth
def grant_role_permission(role_name: str):
    try:
        data = request.get_json()
        if not data or 'actions' not in data or 'resource' not in data:
            return error_response("Permission is required", 400)
        actions: list = data['actions']
        resource: str = data['resource']
        res = RoleMgr.grant_role_permission(role_name, actions, resource)
        return success_response(res)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/roles/<role_name>/permission', methods=['DELETE'])
@login_required
@check_admin_auth
def revoke_role_permission(role_name: str):
    try:
        data = request.get_json()
        if not data or 'actions' not in data or 'resource' not in data:
            return error_response("Permission is required", 400)
        actions: list = data['actions']
        resource: str = data['resource']
        res = RoleMgr.revoke_role_permission(role_name, actions, resource)
        return success_response(res)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users/<user_name>/role', methods=['PUT'])
@login_required
@check_admin_auth
def update_user_role(user_name: str):
    try:
        data = request.get_json()
        if not data or 'role_name' not in data:
            return error_response("Role name is required", 400)
        role_name: str = data['role_name']
        res = RoleMgr.update_user_role(user_name, role_name)
        return success_response(res)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users/<user_name>/permission', methods=['GET'])
@login_required
@check_admin_auth
def get_user_permission(user_name: str):
    try:
        res = RoleMgr.get_user_permission(user_name)
        return success_response(res)
    except Exception as e:
        return error_response(str(e), 500)
