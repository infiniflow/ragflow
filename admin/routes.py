from flask import Blueprint, request
from auth import login_verify
from responses import success_response, error_response
from services import UserMgr, ServiceMgr
from exceptions import AdminException

admin_bp = Blueprint('admin', __name__, url_prefix='/api/v1/admin')


@admin_bp.route('/auth', methods=['GET'])
@login_verify
def auth_admin():
    try:
        return success_response(None, "Admin is authorized", 0)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users', methods=['GET'])
@login_verify
def list_users():
    try:
        users = UserMgr.get_all_users()
        return success_response(users, "Get all users", 0)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users', methods=['POST'])
@login_verify
def create_user():
    try:
        data = request.get_json()
        if not data or 'username' not in data or 'password' not in data:
            return error_response("Username and password are required", 400)

        username = data['username']
        password = data['password']
        role = data.get('role', 'user')

        user = UserMgr.create_user(username, password, role)
        return success_response(user, "User created successfully", 201)

    except AdminException as e:
        return error_response(e.message, e.code)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users/<username>', methods=['DELETE'])
@login_verify
def delete_user(username):
    try:
        UserMgr.delete_user(username)
        return success_response(None, "User and all data deleted successfully")

    except AdminException as e:
        return error_response(e.message, e.code)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users/<username>/password', methods=['PUT'])
@login_verify
def change_password(username):
    try:
        data = request.get_json()
        if not data or 'new_password' not in data:
            return error_response("New password is required", 400)

        new_password = data['new_password']
        UserMgr.update_user_password(username, new_password)
        return success_response(None, "Password updated successfully")

    except AdminException as e:
        return error_response(e.message, e.code)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/users/<username>', methods=['GET'])
@login_verify
def get_user_details(username):
    try:
        user_details = UserMgr.get_user_details(username)
        return success_response(user_details)

    except AdminException as e:
        return error_response(e.message, e.code)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/services', methods=['GET'])
@login_verify
def get_services():
    try:
        services = ServiceMgr.get_all_services()
        return success_response(services, "Get all services", 0)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/service_types/<service_type>', methods=['GET'])
@login_verify
def get_services_by_type(service_type_str):
    try:
        services = ServiceMgr.get_services_by_type(service_type_str)
        return success_response(services)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/services/<service_id>', methods=['GET'])
@login_verify
def get_service(service_id):
    try:
        services = ServiceMgr.get_service_details(service_id)
        return success_response(services)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/services/<service_id>', methods=['DELETE'])
@login_verify
def shutdown_service(service_id):
    try:
        services = ServiceMgr.shutdown_service(service_id)
        return success_response(services)
    except Exception as e:
        return error_response(str(e), 500)


@admin_bp.route('/services/<service_id>', methods=['PUT'])
@login_verify
def restart_service(service_id):
    try:
        services = ServiceMgr.restart_service(service_id)
        return success_response(services)
    except Exception as e:
        return error_response(str(e), 500)
