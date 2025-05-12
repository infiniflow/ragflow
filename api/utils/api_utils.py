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
import logging
import random
import time
from base64 import b64encode
from copy import deepcopy
from functools import wraps
from hmac import HMAC
from io import BytesIO
from urllib.parse import quote, urlencode
from uuid import uuid1

import requests
from flask import (
    Response,
    jsonify,
    make_response,
    send_file,
)
from flask import (
    request as flask_request,
)
from itsdangerous import URLSafeTimedSerializer
from peewee import OperationalError
from werkzeug.http import HTTP_STATUS_CODES

from api import settings
from api.constants import REQUEST_MAX_WAIT_SEC, REQUEST_WAIT_SEC
from api.db.db_models import APIToken
from api.db.services.llm_service import LLMService, TenantLLMService
from api.utils import CustomJSONEncoder, get_uuid, json_dumps

requests.models.complexjson.dumps = functools.partial(json.dumps, cls=CustomJSONEncoder)


def request(**kwargs):
    sess = requests.Session()
    stream = kwargs.pop("stream", sess.stream)
    timeout = kwargs.pop("timeout", None)
    kwargs["headers"] = {k.replace("_", "-").upper(): v for k, v in kwargs.get("headers", {}).items()}
    prepped = requests.Request(**kwargs).prepare()

    if settings.CLIENT_AUTHENTICATION and settings.HTTP_APP_KEY and settings.SECRET_KEY:
        timestamp = str(round(time() * 1000))
        nonce = str(uuid1())
        signature = b64encode(
            HMAC(
                settings.SECRET_KEY.encode("ascii"),
                b"\n".join(
                    [
                        timestamp.encode("ascii"),
                        nonce.encode("ascii"),
                        settings.HTTP_APP_KEY.encode("ascii"),
                        prepped.path_url.encode("ascii"),
                        prepped.body if kwargs.get("json") else b"",
                        urlencode(sorted(kwargs["data"].items()), quote_via=quote, safe="-._~").encode("ascii") if kwargs.get("data") and isinstance(kwargs["data"], dict) else b"",
                    ]
                ),
                "sha1",
            ).digest()
        ).decode("ascii")

        prepped.headers.update(
            {
                "TIMESTAMP": timestamp,
                "NONCE": nonce,
                "APP-KEY": settings.HTTP_APP_KEY,
                "SIGNATURE": signature,
            }
        )

    return sess.send(prepped, stream=stream, timeout=timeout)


def get_exponential_backoff_interval(retries, full_jitter=False):
    """Calculate the exponential backoff wait time."""
    # Will be zero if factor equals 0
    countdown = min(REQUEST_MAX_WAIT_SEC, REQUEST_WAIT_SEC * (2**retries))
    # Full jitter according to
    # https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
    if full_jitter:
        countdown = random.randrange(countdown + 1)
    # Adjust according to maximum wait time and account for negative values.
    return max(0, countdown)


def get_data_error_result(code=settings.RetCode.DATA_ERROR, message="Sorry! Data missing!"):
    logging.exception(Exception(message))
    result_dict = {"code": code, "message": message}
    response = {}
    for key, value in result_dict.items():
        if value is None and key != "code":
            continue
        else:
            response[key] = value
    return jsonify(response)


def server_error_response(e):
    logging.exception(e)
    try:
        if e.code == 401:
            return get_json_result(code=401, message=repr(e))
    except BaseException:
        pass
    if len(e.args) > 1:
        return get_json_result(code=settings.RetCode.EXCEPTION_ERROR, message=repr(e.args[0]), data=e.args[1])
    if repr(e).find("index_not_found_exception") >= 0:
        return get_json_result(code=settings.RetCode.EXCEPTION_ERROR, message="No chunk found, please upload file and parse it.")

    return get_json_result(code=settings.RetCode.EXCEPTION_ERROR, message=repr(e))


def error_response(response_code, message=None):
    if message is None:
        message = HTTP_STATUS_CODES.get(response_code, "Unknown Error")

    return Response(
        json.dumps(
            {
                "message": message,
                "code": response_code,
            }
        ),
        status=response_code,
        mimetype="application/json",
    )


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
                    error_string += "required argument are missing: {}; ".format(",".join(no_arguments))
                if error_arguments:
                    error_string += "required argument values: {}".format(",".join(["{}={}".format(a[0], a[1]) for a in error_arguments]))
                return get_json_result(code=settings.RetCode.ARGUMENT_ERROR, message=error_string)
            return func(*_args, **_kwargs)

        return decorated_function

    return wrapper


def not_allowed_parameters(*params):
    def decorator(f):
        def wrapper(*args, **kwargs):
            input_arguments = flask_request.json or flask_request.form.to_dict()
            for param in params:
                if param in input_arguments:
                    return get_json_result(code=settings.RetCode.ARGUMENT_ERROR, message=f"Parameter {param} isn't allowed")
            return f(*args, **kwargs)

        return wrapper

    return decorator


def is_localhost(ip):
    return ip in {"127.0.0.1", "::1", "[::1]", "localhost"}


def send_file_in_mem(data, filename):
    if not isinstance(data, (str, bytes)):
        data = json_dumps(data)
    if isinstance(data, str):
        data = data.encode("utf-8")

    f = BytesIO()
    f.write(data)
    f.seek(0)

    return send_file(f, as_attachment=True, attachment_filename=filename)


def get_json_result(code=settings.RetCode.SUCCESS, message="success", data=None):
    response = {"code": code, "message": message, "data": data}
    return jsonify(response)


def apikey_required(func):
    @wraps(func)
    def decorated_function(*args, **kwargs):
        token = flask_request.headers.get("Authorization").split()[1]
        objs = APIToken.query(token=token)
        if not objs:
            return build_error_result(message="API-KEY is invalid!", code=settings.RetCode.FORBIDDEN)
        kwargs["tenant_id"] = objs[0].tenant_id
        return func(*args, **kwargs)

    return decorated_function


def build_error_result(code=settings.RetCode.FORBIDDEN, message="success"):
    response = {"code": code, "message": message}
    response = jsonify(response)
    response.status_code = code
    return response


def construct_response(code=settings.RetCode.SUCCESS, message="success", data=None, auth=None):
    result_dict = {"code": code, "message": message, "data": data}
    response_dict = {}
    for key, value in result_dict.items():
        if value is None and key != "code":
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


def construct_result(code=settings.RetCode.DATA_ERROR, message="data is missing"):
    result_dict = {"code": code, "message": message}
    response = {}
    for key, value in result_dict.items():
        if value is None and key != "code":
            continue
        else:
            response[key] = value
    return jsonify(response)


def construct_json_result(code=settings.RetCode.SUCCESS, message="success", data=None):
    if data is None:
        return jsonify({"code": code, "message": message})
    else:
        return jsonify({"code": code, "message": message, "data": data})


def construct_error_response(e):
    logging.exception(e)
    try:
        if e.code == 401:
            return construct_json_result(code=settings.RetCode.UNAUTHORIZED, message=repr(e))
    except BaseException:
        pass
    if len(e.args) > 1:
        return construct_json_result(code=settings.RetCode.EXCEPTION_ERROR, message=repr(e.args[0]), data=e.args[1])
    return construct_json_result(code=settings.RetCode.EXCEPTION_ERROR, message=repr(e))


def token_required(func):
    @wraps(func)
    def decorated_function(*args, **kwargs):
        authorization_str = flask_request.headers.get("Authorization")
        if not authorization_str:
            return get_json_result(data=False, message="`Authorization` can't be empty")
        authorization_list = authorization_str.split()
        if len(authorization_list) < 2:
            return get_json_result(data=False, message="Please check your authorization format.")
        token = authorization_list[1]
        objs = APIToken.query(token=token)
        if not objs:
            return get_json_result(data=False, message="Authentication error: API key is invalid!", code=settings.RetCode.AUTHENTICATION_ERROR)
        kwargs["tenant_id"] = objs[0].tenant_id
        return func(*args, **kwargs)

    return decorated_function


def get_result(code=settings.RetCode.SUCCESS, message="", data=None):
    if code == 0:
        if data is not None:
            response = {"code": code, "data": data}
        else:
            response = {"code": code}
    else:
        response = {"code": code, "message": message}
    return jsonify(response)


def get_error_data_result(
    message="Sorry! Data missing!",
    code=settings.RetCode.DATA_ERROR,
):
    result_dict = {"code": code, "message": message}
    response = {}
    for key, value in result_dict.items():
        if value is None and key != "code":
            continue
        else:
            response[key] = value
    return jsonify(response)


def get_error_argument_result(message="Invalid arguments"):
    return get_result(code=settings.RetCode.ARGUMENT_ERROR, message=message)


def generate_confirmation_token(tenant_id):
    serializer = URLSafeTimedSerializer(tenant_id)
    return "ragflow-" + serializer.dumps(get_uuid(), salt=tenant_id)[2:34]


def get_parser_config(chunk_method, parser_config):
    if parser_config:
        return parser_config
    if not chunk_method:
        chunk_method = "naive"
    key_mapping = {
        "naive": {"chunk_token_num": 128, "delimiter": r"\n", "html4excel": False, "layout_recognize": "DeepDOC", "raptor": {"use_raptor": False}},
        "qa": {"raptor": {"use_raptor": False}},
        "tag": None,
        "resume": None,
        "manual": {"raptor": {"use_raptor": False}},
        "table": None,
        "paper": {"raptor": {"use_raptor": False}},
        "book": {"raptor": {"use_raptor": False}},
        "laws": {"raptor": {"use_raptor": False}},
        "presentation": {"raptor": {"use_raptor": False}},
        "one": None,
        "knowledge_graph": {"chunk_token_num": 8192, "delimiter": r"\n", "entity_types": ["organization", "person", "location", "event", "time"]},
        "email": None,
        "picture": None,
    }
    parser_config = key_mapping[chunk_method]
    return parser_config


def get_data_openai(
    id=None,
    created=None,
    model=None,
    prompt_tokens=0,
    completion_tokens=0,
    content=None,
    finish_reason=None,
    object="chat.completion",
    param=None,
):
    total_tokens = prompt_tokens + completion_tokens
    return {
        "id": f"{id}",
        "object": object,
        "created": int(time.time()) if created else None,
        "model": model,
        "param": param,
        "usage": {
            "prompt_tokens": prompt_tokens,
            "completion_tokens": completion_tokens,
            "total_tokens": total_tokens,
            "completion_tokens_details": {"reasoning_tokens": 0, "accepted_prediction_tokens": 0, "rejected_prediction_tokens": 0},
        },
        "choices": [{"message": {"role": "assistant", "content": content}, "logprobs": None, "finish_reason": finish_reason, "index": 0}],
    }


def check_duplicate_ids(ids, id_type="item"):
    """
    Check for duplicate IDs in a list and return unique IDs and error messages.

    Args:
        ids (list): List of IDs to check for duplicates
        id_type (str): Type of ID for error messages (e.g., 'document', 'dataset', 'chunk')

    Returns:
        tuple: (unique_ids, error_messages)
            - unique_ids (list): List of unique IDs
            - error_messages (list): List of error messages for duplicate IDs
    """
    id_count = {}
    duplicate_messages = []

    # Count occurrences of each ID
    for id_value in ids:
        id_count[id_value] = id_count.get(id_value, 0) + 1

    # Check for duplicates
    for id_value, count in id_count.items():
        if count > 1:
            duplicate_messages.append(f"Duplicate {id_type} ids: {id_value}")

    # Return unique IDs and error messages
    return list(set(ids)), duplicate_messages


def verify_embedding_availability(embd_id: str, tenant_id: str) -> tuple[bool, Response | None]:
    """
    Verifies availability of an embedding model for a specific tenant.

    Implements a four-stage validation process:
    1. Model identifier parsing and validation
    2. System support verification
    3. Tenant authorization check
    4. Database operation error handling

    Args:
        embd_id (str): Unique identifier for the embedding model in format "model_name@factory"
        tenant_id (str): Tenant identifier for access control

    Returns:
        tuple[bool, Response | None]:
        - First element (bool):
            - True: Model is available and authorized
            - False: Validation failed
        - Second element contains:
            - None on success
            - Error detail dict on failure

    Raises:
        ValueError: When model identifier format is invalid
        OperationalError: When database connection fails (auto-handled)

    Examples:
        >>> verify_embedding_availability("text-embedding@openai", "tenant_123")
        (True, None)

        >>> verify_embedding_availability("invalid_model", "tenant_123")
        (False, {'code': 101, 'message': "Unsupported model: <invalid_model>"})
    """
    try:
        llm_name, llm_factory = TenantLLMService.split_model_name_and_factory(embd_id)
        if not LLMService.query(llm_name=llm_name, fid=llm_factory, model_type="embedding"):
            return False, get_error_argument_result(f"Unsupported model: <{embd_id}>")

        # Tongyi-Qianwen is added to TenantLLM by default, but remains unusable with empty api_key
        tenant_llms = TenantLLMService.get_my_llms(tenant_id=tenant_id)
        is_tenant_model = any(llm["llm_name"] == llm_name and llm["llm_factory"] == llm_factory and llm["model_type"] == "embedding" for llm in tenant_llms)

        is_builtin_model = embd_id in settings.BUILTIN_EMBEDDING_MODELS
        if not (is_builtin_model or is_tenant_model):
            return False, get_error_argument_result(f"Unauthorized model: <{embd_id}>")
    except OperationalError as e:
        logging.exception(e)
        return False, get_error_data_result(message="Database operation failed")

    return True, None


def deep_merge(default: dict, custom: dict) -> dict:
    """
    Recursively merges two dictionaries with priority given to `custom` values.

    Creates a deep copy of the `default` dictionary and iteratively merges nested
    dictionaries using a stack-based approach. Non-dict values in `custom` will
    completely override corresponding entries in `default`.

    Args:
        default (dict): Base dictionary containing default values.
        custom (dict): Dictionary containing overriding values.

    Returns:
        dict: New merged dictionary combining values from both inputs.

    Example:
        >>> from copy import deepcopy
        >>> default = {"a": 1, "nested": {"x": 10, "y": 20}}
        >>> custom = {"b": 2, "nested": {"y": 99, "z": 30}}
        >>> deep_merge(default, custom)
        {'a': 1, 'b': 2, 'nested': {'x': 10, 'y': 99, 'z': 30}}

        >>> deep_merge({"config": {"mode": "auto"}}, {"config": "manual"})
        {'config': 'manual'}

    Notes:
        1. Merge priority is always given to `custom` values at all nesting levels
        2. Non-dict values (e.g. list, str) in `custom` will replace entire values
           in `default`, even if the original value was a dictionary
        3. Time complexity: O(N) where N is total key-value pairs in `custom`
        4. Recommended for configuration merging and nested data updates
    """
    merged = deepcopy(default)
    stack = [(merged, custom)]

    while stack:
        base_dict, override_dict = stack.pop()

        for key, val in override_dict.items():
            if key in base_dict and isinstance(val, dict) and isinstance(base_dict[key], dict):
                stack.append((base_dict[key], val))
            else:
                base_dict[key] = val

    return merged
