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
import json
import re
from datetime import datetime

from flask import request, session, redirect
from werkzeug.security import generate_password_hash, check_password_hash
from flask_login import login_required, current_user, login_user, logout_user

from api.db.db_models import TenantLLM
from api.db.services.llm_service import TenantLLMService, LLMService
from api.utils.api_utils import server_error_response, validate_request, get_data_error_result
from api.utils import get_uuid, get_format_time, decrypt, download_img, current_timestamp, datetime_format
from api.db import UserTenantRole, LLMType, FileType
from api.settings import RetCode, GITHUB_OAUTH, FEISHU_OAUTH, CHAT_MDL, EMBEDDING_MDL, ASR_MDL, IMAGE2TEXT_MDL, PARSERS, \
    API_KEY, \
    LLM_FACTORY, LLM_BASE_URL, RERANK_MDL
from api.db.services.user_service import UserService, TenantService, UserTenantService
from api.db.services.file_service import FileService
from api.settings import stat_logger
from api.utils.api_utils import get_json_result, construct_response


@manager.route('/login', methods=['POST', 'GET'])
def login():
    if not request.json:
        return get_json_result(data=False,
                               retcode=RetCode.AUTHENTICATION_ERROR,
                               retmsg='Unauthorized!')

    email = request.json.get('email', "")
    users = UserService.query(email=email)
    if not users:
        return get_json_result(data=False,
                               retcode=RetCode.AUTHENTICATION_ERROR,
                               retmsg=f'Email: {email} is not registered!')

    password = request.json.get('password')
    try:
        password = decrypt(password)
    except BaseException:
        return get_json_result(data=False,
                               retcode=RetCode.SERVER_ERROR,
                               retmsg='Fail to crypt password')

    user = UserService.query_user(email, password)
    if user:
        response_data = user.to_json()
        user.access_token = get_uuid()
        login_user(user)
        user.update_time = current_timestamp(),
        user.update_date = datetime_format(datetime.now()),
        user.save()
        msg = "Welcome back!"
        return construct_response(data=response_data, auth=user.get_id(), retmsg=msg)
    else:
        return get_json_result(data=False,
                               retcode=RetCode.AUTHENTICATION_ERROR,
                               retmsg='Email and password do not match!')


@manager.route('/github_callback', methods=['GET'])
def github_callback():
    import requests
    res = requests.post(GITHUB_OAUTH.get("url"),
                        data={
                            "client_id": GITHUB_OAUTH.get("client_id"),
                            "client_secret": GITHUB_OAUTH.get("secret_key"),
                            "code": request.args.get('code')},
                        headers={"Accept": "application/json"})
    res = res.json()
    if "error" in res:
        return redirect("/?error=%s" % res["error_description"])

    if "user:email" not in res["scope"].split(","):
        return redirect("/?error=user:email not in scope")

    session["access_token"] = res["access_token"]
    session["access_token_from"] = "github"
    user_info = user_info_from_github(session["access_token"])
    email_address = user_info["email"]
    users = UserService.query(email=email_address)
    user_id = get_uuid()
    if not users:
        # User isn't try to register
        try:
            try:
                avatar = download_img(user_info["avatar_url"])
            except Exception as e:
                stat_logger.exception(e)
                avatar = ""
            users = user_register(user_id, {
                "access_token": session["access_token"],
                "email": email_address,
                "avatar": avatar,
                "nickname": user_info["login"],
                "login_channel": "github",
                "last_login_time": get_format_time(),
                "is_superuser": False,
            })
            if not users:
                raise Exception(f'Fail to register {email_address}.')
            if len(users) > 1:
                raise Exception(f'Same email: {email_address} exists!')

            # Try to log in
            user = users[0]
            login_user(user)
            return redirect("/?auth=%s" % user.get_id())
        except Exception as e:
            rollback_user_registration(user_id)
            stat_logger.exception(e)
            return redirect("/?error=%s" % str(e))

    # User has already registered, try to log in
    user = users[0]
    user.access_token = get_uuid()
    login_user(user)
    user.save()
    return redirect("/?auth=%s" % user.get_id())


@manager.route('/feishu_callback', methods=['GET'])
def feishu_callback():
    import requests
    app_access_token_res = requests.post(FEISHU_OAUTH.get("app_access_token_url"),
                                         data=json.dumps({
                                             "app_id": FEISHU_OAUTH.get("app_id"),
                                             "app_secret": FEISHU_OAUTH.get("app_secret")
                                         }),
                                         headers={"Content-Type": "application/json; charset=utf-8"})
    app_access_token_res = app_access_token_res.json()
    if app_access_token_res['code'] != 0:
        return redirect("/?error=%s" % app_access_token_res)

    res = requests.post(FEISHU_OAUTH.get("user_access_token_url"),
                        data=json.dumps({
                            "grant_type": FEISHU_OAUTH.get("grant_type"),
                            "code": request.args.get('code')
                        }),
                        headers={
                            "Content-Type": "application/json; charset=utf-8",
                            'Authorization': f"Bearer {app_access_token_res['app_access_token']}"
                        })
    res = res.json()
    if res['code'] != 0:
        return redirect("/?error=%s" % res["message"])

    if "contact:user.email:readonly" not in res["data"]["scope"].split(" "):
        return redirect("/?error=contact:user.email:readonly not in scope")
    session["access_token"] = res["data"]["access_token"]
    session["access_token_from"] = "feishu"
    user_info = user_info_from_feishu(session["access_token"])
    email_address = user_info["email"]
    users = UserService.query(email=email_address)
    user_id = get_uuid()
    if not users:
        # User isn't try to register
        try:
            try:
                avatar = download_img(user_info["avatar_url"])
            except Exception as e:
                stat_logger.exception(e)
                avatar = ""
            users = user_register(user_id, {
                "access_token": session["access_token"],
                "email": email_address,
                "avatar": avatar,
                "nickname": user_info["en_name"],
                "login_channel": "feishu",
                "last_login_time": get_format_time(),
                "is_superuser": False,
            })
            if not users:
                raise Exception(f'Fail to register {email_address}.')
            if len(users) > 1:
                raise Exception(f'Same email: {email_address} exists!')

            # Try to log in
            user = users[0]
            login_user(user)
            return redirect("/?auth=%s" % user.get_id())
        except Exception as e:
            rollback_user_registration(user_id)
            stat_logger.exception(e)
            return redirect("/?error=%s" % str(e))

    # User has already registered, try to log in
    user = users[0]
    user.access_token = get_uuid()
    login_user(user)
    user.save()
    return redirect("/?auth=%s" % user.get_id())


def user_info_from_feishu(access_token):
    import requests
    headers = {"Content-Type": "application/json; charset=utf-8",
               'Authorization': f"Bearer {access_token}"}
    res = requests.get(
        f"https://open.feishu.cn/open-apis/authen/v1/user_info",
        headers=headers)
    user_info = res.json()["data"]
    user_info["email"] = None if user_info.get("email") == "" else user_info["email"]
    return user_info


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
            return get_json_result(data=False, retcode=RetCode.AUTHENTICATION_ERROR, retmsg='Password error!')

        if new_password:
            update_dict["password"] = generate_password_hash(decrypt(new_password))

    for k in request_data.keys():
        if k in ["password", "new_password", "email", "status", "is_superuser", "login_channel", "is_anonymous",
                 "is_active", "is_authenticated", "last_login_time"]:
            continue
        update_dict[k] = request_data[k]

    try:
        UserService.update_by_id(current_user.id, update_dict)
        return get_json_result(data=True)
    except Exception as e:
        stat_logger.exception(e)
        return get_json_result(data=False, retmsg='Update failure!', retcode=RetCode.EXCEPTION_ERROR)


@manager.route("/info", methods=["GET"])
@login_required
def user_profile():
    return get_json_result(data=current_user.to_dict())


def rollback_user_registration(user_id):
    try:
        UserService.delete_by_id(user_id)
    except Exception as e:
        pass
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
        TenantLLM.delete().where(TenantLLM.tenant_id == user_id).execute()
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
        "img2txt_id": IMAGE2TEXT_MDL,
        "rerank_id": RERANK_MDL
    }
    usr_tenant = {
        "tenant_id": user_id,
        "user_id": user_id,
        "invited_by": user_id,
        "role": UserTenantRole.OWNER
    }
    file_id = get_uuid()
    file = {
        "id": file_id,
        "parent_id": file_id,
        "tenant_id": user_id,
        "created_by": user_id,
        "name": "/",
        "type": FileType.FOLDER.value,
        "size": 0,
        "location": "",
    }
    tenant_llm = []
    for llm in LLMService.query(fid=LLM_FACTORY):
        tenant_llm.append({"tenant_id": user_id,
                           "llm_factory": LLM_FACTORY,
                           "llm_name": llm.llm_name,
                           "model_type": llm.model_type,
                           "api_key": API_KEY,
                           "api_base": LLM_BASE_URL
                           })

    if not UserService.save(**user):
        return
    TenantService.insert(**tenant)
    UserTenantService.insert(**usr_tenant)
    TenantLLMService.insert_many(tenant_llm)
    FileService.insert(file)
    return UserService.query(email=user["email"])


@manager.route("/register", methods=["POST"])
@validate_request("nickname", "email", "password")
def user_add():
    req = request.json
    email_address = req["email"]

    # Validate the email address
    if not re.match(r"^[\w\._-]+@([\w_-]+\.)+[\w-]{2,5}$", email_address):
        return get_json_result(data=False,
                               retmsg=f'Invalid email address: {email_address}!',
                               retcode=RetCode.OPERATING_ERROR)

    # Check if the email address is already used
    if UserService.query(email=email_address):
        return get_json_result(
            data=False,
            retmsg=f'Email: {email_address} has already registered!',
            retcode=RetCode.OPERATING_ERROR)

    # Construct user info data
    nickname = req["nickname"]
    user_dict = {
        "access_token": get_uuid(),
        "email": email_address,
        "nickname": nickname,
        "password": decrypt(req["password"]),
        "login_channel": "password",
        "last_login_time": get_format_time(),
        "is_superuser": False,
    }

    user_id = get_uuid()
    try:
        users = user_register(user_id, user_dict)
        if not users:
            raise Exception(f'Fail to register {email_address}.')
        if len(users) > 1:
            raise Exception(f'Same email: {email_address} exists!')
        user = users[0]
        login_user(user)
        return construct_response(data=user.to_json(),
                                  auth=user.get_id(),
                                  retmsg=f"{nickname}, welcome aboard!")
    except Exception as e:
        rollback_user_registration(user_id)
        stat_logger.exception(e)
        return get_json_result(data=False,
                               retmsg=f'User registration failure, error: {str(e)}',
                               retcode=RetCode.EXCEPTION_ERROR)


@manager.route("/tenant_info", methods=["GET"])
@login_required
def tenant_info():
    try:
        tenants = TenantService.get_info_by(current_user.id)
        if not tenants:
            return get_data_error_result(retmsg="Tenant not found!")
        return get_json_result(data=tenants[0])
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
