#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import re

from flask import request, session, redirect, url_for
from werkzeug.security import generate_password_hash, check_password_hash
from flask_login import login_required, current_user, login_user, logout_user

from api.db.db_models import TenantLLM
from api.db.services.llm_service import TenantLLMService, LLMService
from api.utils.api_utils import server_error_response, validate_request
from api.utils import get_uuid, get_format_time, decrypt, download_img
from api.db import UserTenantRole, LLMType
from api.settings import RetCode, GITHUB_OAUTH, CHAT_MDL, EMBEDDING_MDL, ASR_MDL, IMAGE2TEXT_MDL, PARSERS, API_KEY, \
    LLM_FACTORY
from api.db.services.user_service import UserService, TenantService, UserTenantService
from api.settings import stat_logger
from api.utils.api_utils import get_json_result, cors_reponse


@manager.route('/login', methods=['POST', 'GET'])
def login():
    login_channel = "password"
    if not request.json:
        return get_json_result(data=False, retcode=RetCode.AUTHENTICATION_ERROR,
                               retmsg='Unautherized!')

    email = request.json.get('email', "")
    users = UserService.query(email=email)
    if not users:
        return get_json_result(
            data=False, retcode=RetCode.AUTHENTICATION_ERROR, retmsg=f'This Email is not registered!')

    password = request.json.get('password')
    try:
        password = decrypt(password)
    except BaseException:
        return get_json_result(
            data=False, retcode=RetCode.SERVER_ERROR, retmsg='Fail to crypt password')

    user = UserService.query_user(email, password)
    if user:
        response_data = user.to_json()
        user.access_token = get_uuid()
        login_user(user)
        user.save()
        msg = "Welcome back!"
        return cors_reponse(data=response_data, auth=user.get_id(), retmsg=msg)
    else:
        return get_json_result(data=False, retcode=RetCode.AUTHENTICATION_ERROR,
                               retmsg='Email and Password do not match!')


@manager.route('/github_callback', methods=['GET'])
def github_callback():
    import requests
    res = requests.post(GITHUB_OAUTH.get("url"), data={
        "client_id": GITHUB_OAUTH.get("client_id"),
        "client_secret": GITHUB_OAUTH.get("secret_key"),
        "code": request.args.get('code')
    }, headers={"Accept": "application/json"})
    res = res.json()
    if "error" in res:
        return redirect("/?error=%s" % res["error_description"])

    if "user:email" not in res["scope"].split(","):
        return redirect("/?error=user:email not in scope")

    session["access_token"] = res["access_token"]
    session["access_token_from"] = "github"
    userinfo = user_info_from_github(session["access_token"])
    users = UserService.query(email=userinfo["email"])
    user_id = get_uuid()
    if not users:
        try:
            try:
                avatar = download_img(userinfo["avatar_url"])
            except Exception as e:
                stat_logger.exception(e)
                avatar = ""
            users = user_register(user_id, {
                "access_token": session["access_token"],
                "email": userinfo["email"],
                "avatar": avatar,
                "nickname": userinfo["login"],
                "login_channel": "github",
                "last_login_time": get_format_time(),
                "is_superuser": False,
            })
            if not users:
                raise Exception('Register user failure.')
            if len(users) > 1:
                raise Exception('Same E-mail exist!')
            user = users[0]
            login_user(user)
            return redirect("/?auth=%s" % user.get_id())
        except Exception as e:
            rollback_user_registration(user_id)
            stat_logger.exception(e)
            return redirect("/?error=%s" % str(e))
    user = users[0]
    user.access_token = get_uuid()
    login_user(user)
    user.save()
    return redirect("/?auth=%s" % user.get_id())


def user_info_from_github(access_token):
    import requests
    headers = {"Accept": "application/json",
               'Authorization': f"token {access_token}"}
    res = requests.get(
        f"https://api.github.com/user?access_token={access_token}",
        headers=headers)
    user_info = res.json()
    email_info = requests.get(
        f"https://api.github.com/user/emails?access_token={access_token}",
        headers=headers).json()
    user_info["email"] = next(
        (email for email in email_info if email['primary'] == True),
        None)["email"]
    return user_info


@manager.route("/logout", methods=['GET'])
@login_required
def log_out():
    current_user.access_token = ""
    current_user.save()
    logout_user()
    return get_json_result(data=True)


@manager.route("/setting", methods=["POST"])
@login_required
def setting_user():
    update_dict = {}
    request_data = request.json
    if request_data.get("password"):
        new_password = request_data.get("new_password")
        if not check_password_hash(
                current_user.password, decrypt(request_data["password"])):
            return get_json_result(
                data=False, retcode=RetCode.AUTHENTICATION_ERROR, retmsg='Password error!')

        if new_password:
            update_dict["password"] = generate_password_hash(
                decrypt(new_password))

    for k in request_data.keys():
        if k in ["password", "new_password"]:
            continue
        update_dict[k] = request_data[k]

    try:
        UserService.update_by_id(current_user.id, update_dict)
        return get_json_result(data=True)
    except Exception as e:
        stat_logger.exception(e)
        return get_json_result(
            data=False, retmsg='Update failure!', retcode=RetCode.EXCEPTION_ERROR)


@manager.route("/info", methods=["GET"])
@login_required
def user_info():
    return get_json_result(data=current_user.to_dict())


def rollback_user_registration(user_id):
    try:
        TenantService.delete_by_id(user_id)
    except Exception as e:
        pass
    try:
        u = UserTenantService.query(tenant_id=user_id)
        if u:
            UserTenantService.delete_by_id(u[0].id)
    except Exception as e:
        pass
    try:
        TenantLLM.delete().where(TenantLLM.tenant_id == user_id).excute()
    except Exception as e:
        pass


def user_register(user_id, user):
    user["id"] = user_id
    tenant = {
        "id": user_id,
        "name": user["nickname"] + "‘s Kingdom",
        "llm_id": CHAT_MDL,
        "embd_id": EMBEDDING_MDL,
        "asr_id": ASR_MDL,
        "parser_ids": PARSERS,
        "img2txt_id": IMAGE2TEXT_MDL
    }
    usr_tenant = {
        "tenant_id": user_id,
        "user_id": user_id,
        "invited_by": user_id,
        "role": UserTenantRole.OWNER
    }
    tenant_llm = []
    for llm in LLMService.query(fid=LLM_FACTORY):
        tenant_llm.append({"tenant_id": user_id,
                           "llm_factory": LLM_FACTORY,
                           "llm_name": llm.llm_name,
                           "model_type": llm.model_type,
                           "api_key": API_KEY})

    if not UserService.save(**user):
        return
    TenantService.insert(**tenant)
    UserTenantService.insert(**usr_tenant)
    TenantLLMService.insert_many(tenant_llm)
    return UserService.query(email=user["email"])


@manager.route("/register", methods=["POST"])
@validate_request("nickname", "email", "password")
def user_add():
    req = request.json
    if UserService.query(email=req["email"]):
        return get_json_result(
            data=False, retmsg=f'Email: {req["email"]} has already registered!', retcode=RetCode.OPERATING_ERROR)
    if not re.match(r"^[\w\._-]+@([\w_-]+\.)+[\w-]{2,4}$", req["email"]):
        return get_json_result(data=False, retmsg=f'Invaliad e-mail: {req["email"]}!',
                               retcode=RetCode.OPERATING_ERROR)

    user_dict = {
        "access_token": get_uuid(),
        "email": req["email"],
        "nickname": req["nickname"],
        "password": decrypt(req["password"]),
        "login_channel": "password",
        "last_login_time": get_format_time(),
        "is_superuser": False,
    }

    user_id = get_uuid()
    try:
        users = user_register(user_id, user_dict)
        if not users:
            raise Exception('Register user failure.')
        if len(users) > 1:
            raise Exception('Same E-mail exist!')
        user = users[0]
        login_user(user)
        return cors_reponse(data=user.to_json(),
                            auth=user.get_id(), retmsg="Welcome aboard!")
    except Exception as e:
        rollback_user_registration(user_id)
        stat_logger.exception(e)
        return get_json_result(
            data=False, retmsg='User registration failure!', retcode=RetCode.EXCEPTION_ERROR)


@manager.route("/tenant_info", methods=["GET"])
@login_required
def tenant_info():
    try:
        tenants = TenantService.get_by_user_id(current_user.id)[0]
        return get_json_result(data=tenants)
    except Exception as e:
        return server_error_response(e)


@manager.route("/set_tenant_info", methods=["POST"])
@login_required
@validate_request("tenant_id", "asr_id", "embd_id", "img2txt_id", "llm_id")
def set_tenant_info():
    req = request.json
    try:
        tid = req["tenant_id"]
        del req["tenant_id"]
        TenantService.update_by_id(tid, req)
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)
