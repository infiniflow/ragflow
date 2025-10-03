import logging
import uuid
from functools import wraps
from flask import request, jsonify

from api.common.exceptions import AdminException
from api.db.init_data import encode_to_base64
from api.db.services import UserService


def check_admin(username: str, password: str):
    users = UserService.query(email=username)
    if not users:
        logging.info(f"Username: {username} is not registered!")
        user_info = {
            "id": uuid.uuid1().hex,
            "password": encode_to_base64("admin"),
            "nickname": "admin",
            "is_superuser": True,
            "email": "admin@ragflow.io",
            "creator": "system",
            "status": "1",
        }
        if not UserService.save(**user_info):
            raise AdminException("Can't init admin.", 500)

    user = UserService.query_user(username, password)
    if user:
        return True
    else:
        return False


def login_verify(f):
    @wraps(f)
    def decorated(*args, **kwargs):
        auth = request.authorization
        if not auth or 'username' not in auth.parameters or 'password' not in auth.parameters:
            return jsonify({
                "code": 401,
                "message": "Authentication required",
                "data": None
            }), 200

        username = auth.parameters['username']
        password = auth.parameters['password']
        # TODO: to check the username and password from DB
        if check_admin(username, password) is False:
            return jsonify({
                "code": 403,
                "message": "Access denied",
                "data": None
            }), 200

        return f(*args, **kwargs)

    return decorated
