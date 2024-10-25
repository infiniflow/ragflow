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
import functools
import json
import random
import time
from base64 import b64encode
from functools import wraps
from hmac import HMAC
from io import BytesIO
from urllib.parse import quote, urlencode
from uuid import uuid1

import requests
from flask import (
    Response, jsonify, send_file, make_response,
    request as flask_request,
)
from itsdangerous import URLSafeTimedSerializer
from werkzeug.http import HTTP_STATUS_CODES

from api.db.db_models import APIToken
from api.settings import (
    REQUEST_MAX_WAIT_SEC, REQUEST_WAIT_SEC,
    stat_logger, CLIENT_AUTHENTICATION, HTTP_APP_KEY, SECRET_KEY
)
from api.settings import RetCode
from api.utils import CustomJSONEncoder, get_uuid
from api.utils import json_dumps

requests.models.complexjson.dumps = functools.partial(
    json.dumps, cls=CustomJSONEncoder)


def request(**kwargs):
    sess = requests.Session()
    stream = kwargs.pop('stream', sess.stream)
    timeout = kwargs.pop('timeout', None)
    kwargs['headers'] = {
        k.replace(
            '_',
            '-').upper(): v for k,
                                v in kwargs.get(
            'headers',
            {}).items()}
    prepped = requests.Request(**kwargs).prepare()

    if CLIENT_AUTHENTICATION and HTTP_APP_KEY and SECRET_KEY:
        timestamp = str(round(time() * 1000))
        nonce = str(uuid1())
        signature = b64encode(HMAC(SECRET_KEY.encode('ascii'), b'\n'.join([
            timestamp.encode('ascii'),
            nonce.encode('ascii'),
            HTTP_APP_KEY.encode('ascii'),
            prepped.path_url.encode('ascii'),
            prepped.body if kwargs.get('json') else b'',
            urlencode(
                sorted(
                    kwargs['data'].items()),
                quote_via=quote,
                safe='-._~').encode('ascii')
            if kwargs.get('data') and isinstance(kwargs['data'], dict) else b'',
        ]), 'sha1').digest()).decode('ascii')

        prepped.headers.update({
            'TIMESTAMP': timestamp,
            'NONCE': nonce,
            'APP-KEY': HTTP_APP_KEY,
            'SIGNATURE': signature,
        })

    return sess.send(prepped, stream=stream, timeout=timeout)


def get_exponential_backoff_interval(retries, full_jitter=False):
    """Calculate the exponential backoff wait time."""
    # Will be zero if factor equals 0
    countdown = min(REQUEST_MAX_WAIT_SEC, REQUEST_WAIT_SEC * (2 ** retries))
    # Full jitter according to
    # https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
    if full_jitter:
        countdown = random.randrange(countdown + 1)
    # Adjust according to maximum wait time and account for negative values.
    return max(0, countdown)


def get_data_error_result(retcode=RetCode.DATA_ERROR,
                          retmsg='Sorry! Data missing!'):
    import re
    result_dict = {
        "retcode": retcode,
        "retmsg": re.sub(
            r"rag",
            "seceum",
            retmsg,
            flags=re.IGNORECASE)}
    response = {}
    for key, value in result_dict.items():
        if value is None and key != "retcode":
            continue
        else:
            response[key] = value
    return jsonify(response)


def server_error_response(e):
    stat_logger.exception(e)
    try:
        if e.code == 401:
            return get_json_result(retcode=401, retmsg=repr(e))
    except BaseException:
        pass
    if len(e.args) > 1:
        return get_json_result(
            retcode=RetCode.EXCEPTION_ERROR, retmsg=repr(e.args[0]), data=e.args[1])
    if repr(e).find("index_not_found_exception") >= 0:
        return get_json_result(retcode=RetCode.EXCEPTION_ERROR,
                               retmsg="No chunk found, please upload file and parse it.")

    return get_json_result(retcode=RetCode.EXCEPTION_ERROR, retmsg=repr(e))


def error_response(response_code, retmsg=None):
    if retmsg is None:
        retmsg = HTTP_STATUS_CODES.get(response_code, 'Unknown Error')

    return Response(json.dumps({
        'retmsg': retmsg,
        'retcode': response_code,
    }), status=response_code, mimetype='application/json')


def validate_request(*args, **kwargs):
    def wrapper(func):
        @wraps(func)
        def decorated_function(*_args, **_kwargs):
            input_arguments = flask_request.json or flask_request.form.to_dict()
            no_arguments = []
            error_arguments = []
            for arg in args:
                if arg not in input_arguments:
                    no_arguments.append(arg)
            for k, v in kwargs.items():
                config_value = input_arguments.get(k, None)
                if config_value is None:
                    no_arguments.append(k)
                elif isinstance(v, (tuple, list)):
                    if config_value not in v:
                        error_arguments.append((k, set(v)))
                elif config_value != v:
                    error_arguments.append((k, v))
            if no_arguments or error_arguments:
                error_string = ""
                if no_arguments:
                    error_string += "required argument are missing: {}; ".format(
                        ",".join(no_arguments))
                if error_arguments:
                    error_string += "required argument values: {}".format(
                        ",".join(["{}={}".format(a[0], a[1]) for a in error_arguments]))
                return get_json_result(
                    retcode=RetCode.ARGUMENT_ERROR, retmsg=error_string)
            return func(*_args, **_kwargs)

        return decorated_function

    return wrapper


def is_localhost(ip):
    return ip in {'127.0.0.1', '::1', '[::1]', 'localhost'}


def send_file_in_mem(data, filename):
    if not isinstance(data, (str, bytes)):
        data = json_dumps(data)
    if isinstance(data, str):
        data = data.encode('utf-8')

    f = BytesIO()
    f.write(data)
    f.seek(0)

    return send_file(f, as_attachment=True, attachment_filename=filename)


def get_json_result(retcode=RetCode.SUCCESS, retmsg='success', data=None):
    response = {"retcode": retcode, "retmsg": retmsg, "data": data}
    return jsonify(response)

def apikey_required(func):
    @wraps(func)
    def decorated_function(*args, **kwargs):
        token = flask_request.headers.get('Authorization').split()[1]
        objs = APIToken.query(token=token)
        if not objs:
            return build_error_result(
                error_msg='API-KEY is invalid!', retcode=RetCode.FORBIDDEN
            )
        kwargs['tenant_id'] = objs[0].tenant_id
        return func(*args, **kwargs)

    return decorated_function


def build_error_result(retcode=RetCode.FORBIDDEN, error_msg='success'):
    response = {"error_code": retcode, "error_msg": error_msg}
    response = jsonify(response)
    response.status_code = retcode
    return response


def construct_response(retcode=RetCode.SUCCESS,
                       retmsg='success', data=None, auth=None):
    result_dict = {"retcode": retcode, "retmsg": retmsg, "data": data}
    response_dict = {}
    for key, value in result_dict.items():
        if value is None and key != "retcode":
            continue
        else:
            response_dict[key] = value
    response = make_response(jsonify(response_dict))
    if auth:
        response.headers["Authorization"] = auth
    response.headers["Access-Control-Allow-Origin"] = "*"
    response.headers["Access-Control-Allow-Method"] = "*"
    response.headers["Access-Control-Allow-Headers"] = "*"
    response.headers["Access-Control-Allow-Headers"] = "*"
    response.headers["Access-Control-Expose-Headers"] = "Authorization"
    return response


def construct_result(code=RetCode.DATA_ERROR, message='data is missing'):
    import re
    result_dict = {"code": code, "message": re.sub(r"rag", "seceum", message, flags=re.IGNORECASE)}
    response = {}
    for key, value in result_dict.items():
        if value is None and key != "code":
            continue
        else:
            response[key] = value
    return jsonify(response)


def construct_json_result(code=RetCode.SUCCESS, message='success', data=None):
    if data is None:
        return jsonify({"code": code, "message": message})
    else:
        return jsonify({"code": code, "message": message, "data": data})


def construct_error_response(e):
    stat_logger.exception(e)
    try:
        if e.code == 401:
            return construct_json_result(code=RetCode.UNAUTHORIZED, message=repr(e))
    except BaseException:
        pass
    if len(e.args) > 1:
        return construct_json_result(code=RetCode.EXCEPTION_ERROR, message=repr(e.args[0]), data=e.args[1])
    if repr(e).find("index_not_found_exception") >= 0:
        return construct_json_result(code=RetCode.EXCEPTION_ERROR,
                                     message="No chunk found, please upload file and parse it.")

    return construct_json_result(code=RetCode.EXCEPTION_ERROR, message=repr(e))


def token_required(func):
    @wraps(func)
    def decorated_function(*args, **kwargs):
        token = flask_request.headers.get('Authorization').split()[1]
        objs = APIToken.query(token=token)
        if not objs:
            return get_json_result(
                data=False, retmsg='Token is not valid!', retcode=RetCode.AUTHENTICATION_ERROR
            )
        kwargs['tenant_id'] = objs[0].tenant_id
        return func(*args, **kwargs)

    return decorated_function


def get_result(retcode=RetCode.SUCCESS, retmsg='error', data=None):
    if retcode == 0:
        if data is not None:
            response = {"code": retcode, "data": data}
        else:
            response = {"code": retcode}
    else:
        response = {"code": retcode, "message": retmsg}
    return jsonify(response)


def get_error_data_result(retmsg='Sorry! Data missing!', retcode=RetCode.DATA_ERROR,
                          ):
    import re
    result_dict = {
        "code": retcode,
        "message": re.sub(
            r"rag",
            "seceum",
            retmsg,
            flags=re.IGNORECASE)}
    response = {}
    for key, value in result_dict.items():
        if value is None and key != "code":
            continue
        else:
            response[key] = value
    return jsonify(response)


def generate_confirmation_token(tenent_id):
    serializer = URLSafeTimedSerializer(tenent_id)
    return "ragflow-" + serializer.dumps(get_uuid(), salt=tenent_id)[2:34]


def valid(permission,valid_permission,language,valid_language,chunk_method,valid_chunk_method):
    if valid_parameter(permission,valid_permission):
        return valid_parameter(permission,valid_permission)
    if valid_parameter(language,valid_language):
        return valid_parameter(language,valid_language)
    if valid_parameter(chunk_method,valid_chunk_method):
        return valid_parameter(chunk_method,valid_chunk_method)

def valid_parameter(parameter,valid_values):
    if parameter and parameter not in valid_values:
       return get_error_data_result(f"`{parameter}` is not in {valid_values}")

def get_parser_config(chunk_method,parser_config):
    if parser_config:
        return parser_config
    if not chunk_method:
        chunk_method = "naive"
    key_mapping={"naive":{"chunk_token_num": 128, "delimiter": "\\n!?;。；！？", "html4excel": False,"layout_recognize": True, "raptor": {"user_raptor": False}},
                 "qa":{"raptor":{"use_raptor":False}},
                 "resume":None,
                 "manual":{"raptor":{"use_raptor":False}},
                 "table":None,
                 "paper":{"raptor":{"use_raptor":False}},
                 "book":{"raptor":{"use_raptor":False}},
                 "laws":{"raptor":{"use_raptor":False}},
                 "presentation":{"raptor":{"use_raptor":False}},
                 "one":None,
                 "knowledge_graph":{"chunk_token_num":8192,"delimiter":"\\n!?;。；！？","entity_types":["organization","person","location","event","time"]}}
    parser_config=key_mapping[chunk_method]
    return parser_config