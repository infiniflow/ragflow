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

import asyncio
import ipaddress
import json
import logging
import time
from typing import Any, cast

import jwt

from agent.canvas import Canvas
from api.db import CanvasCategory
from api.db.services.canvas_service import UserCanvasService
from api.db.services.file_service import FileService
from api.db.services.user_canvas_version import UserCanvasVersionService
from common.constants import RetCode
from common.misc_utils import get_uuid
from api.utils.api_utils import get_data_error_result, get_error_data_result, get_json_result, get_request_json, token_required
from api.utils.api_utils import get_result
from quart import request, Response


@manager.route('/agents', methods=['GET'])  # noqa: F821
@token_required
def list_agents(tenant_id):
    id = request.args.get("id")
    title = request.args.get("title")
    if id or title:
        canvas = UserCanvasService.query(id=id, title=title, user_id=tenant_id)
        if not canvas:
            return get_error_data_result("The agent doesn't exist.")
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 30))
    order_by = request.args.get("orderby", "update_time")
    if request.args.get("desc") == "False" or request.args.get("desc") == "false":
        desc = False
    else:
        desc = True
    canvas = UserCanvasService.get_list(tenant_id, page_number, items_per_page, order_by, desc, id, title)
    return get_result(data=canvas)


@manager.route("/agents", methods=["POST"])  # noqa: F821
@token_required
async def create_agent(tenant_id: str):
    req: dict[str, Any] = cast(dict[str, Any], await get_request_json())
    req["user_id"] = tenant_id

    if req.get("dsl") is not None:
        if not isinstance(req["dsl"], str):
            req["dsl"] = json.dumps(req["dsl"], ensure_ascii=False)

        req["dsl"] = json.loads(req["dsl"])
    else:
        return get_json_result(data=False, message="No DSL data in request.", code=RetCode.ARGUMENT_ERROR)

    if req.get("title") is not None:
        req["title"] = req["title"].strip()
    else:
        return get_json_result(data=False, message="No title in request.", code=RetCode.ARGUMENT_ERROR)

    if UserCanvasService.query(user_id=tenant_id, title=req["title"]):
        return get_data_error_result(message=f"Agent with title {req['title']} already exists.")

    agent_id = get_uuid()
    req["id"] = agent_id

    if not UserCanvasService.save(**req):
        return get_data_error_result(message="Fail to create agent.")

    UserCanvasVersionService.insert(
        user_canvas_id=agent_id,
        title="{0}_{1}".format(req["title"], time.strftime("%Y_%m_%d_%H_%M_%S")),
        dsl=req["dsl"]
    )

    return get_json_result(data=True)


@manager.route("/agents/<agent_id>", methods=["PUT"])  # noqa: F821
@token_required
async def update_agent(tenant_id: str, agent_id: str):
    req: dict[str, Any] = {k: v for k, v in cast(dict[str, Any], (await get_request_json())).items() if v is not None}
    req["user_id"] = tenant_id

    if req.get("dsl") is not None:
        if not isinstance(req["dsl"], str):
            req["dsl"] = json.dumps(req["dsl"], ensure_ascii=False)

        req["dsl"] = json.loads(req["dsl"])

    if req.get("title") is not None:
        req["title"] = req["title"].strip()

    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        return get_json_result(
            data=False, message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR)

    UserCanvasService.update_by_id(agent_id, req)

    if req.get("dsl") is not None:
        UserCanvasVersionService.insert(
            user_canvas_id=agent_id,
            title="{0}_{1}".format(req["title"], time.strftime("%Y_%m_%d_%H_%M_%S")),
            dsl=req["dsl"]
        )

        UserCanvasVersionService.delete_all_versions(agent_id)

    return get_json_result(data=True)


@manager.route("/agents/<agent_id>", methods=["DELETE"])  # noqa: F821
@token_required
def delete_agent(tenant_id: str, agent_id: str):
    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        return get_json_result(
            data=False, message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR)

    UserCanvasService.delete_by_id(agent_id)
    return get_json_result(data=True)

_rate_limit_cache = {}

@manager.route('/webhook/<agent_id>', methods=["POST", "GET", "PUT", "PATCH", "DELETE", "HEAD"])  # noqa: F821
async def webhook(agent_id: str):
    # 1. Fetch canvas by agent_id
    exists, cvs = UserCanvasService.get_by_id(agent_id)
    if not exists:
        return get_data_error_result(message="Canvas not found.")

    # 2. Check canvas category
    if cvs.canvas_category == CanvasCategory.DataFlow:
        return get_data_error_result(message="Dataflow can not be triggered by webhook.")

    # 3. Load DSL from canvas
    dsl = getattr(cvs, "dsl", None)
    if not isinstance(dsl, dict):
        return get_data_error_result(message="Invalid DSL format.")

    # 4. Check webhook configuration in DSL
    components = dsl.get("components", {})
    for k, _ in components.items():
        cpn_obj = components[k]["obj"]
        if cpn_obj["component_name"].lower() == "begin" and cpn_obj["params"]["mode"] == "Webhook":
            webhook_cfg = cpn_obj["params"]

    if not webhook_cfg:
        return get_data_error_result(message="Webhook not configured for this agent.")

    # 5. Validate request method against webhook_cfg.methods
    allowed_methods = webhook_cfg.get("methods", [])
    request_method = request.method.upper()
    if allowed_methods and request_method not in allowed_methods:
        return get_data_error_result(
            message=f"HTTP method '{request_method}' not allowed for this webhook."
        )

    # 6. Validate webhook security
    async def validate_webhook_security(security_cfg: dict):
        """Validate webhook security rules based on security configuration."""

        if not security_cfg:
            return  # No security config → allowed by default

        # 1. Validate max body size
        await _validate_max_body_size(security_cfg)

        # 2. Validate IP whitelist
        _validate_ip_whitelist(security_cfg)

        # # 3. Validate rate limiting
        _validate_rate_limit(security_cfg)

        # 4. Validate authentication
        auth_type = security_cfg.get("auth_type", "none")

        if auth_type == "none":
            return

        if auth_type == "token":
            _validate_token_auth(security_cfg)

        elif auth_type == "basic":
            _validate_basic_auth(security_cfg)

        elif auth_type == "jwt":
            _validate_jwt_auth(security_cfg)

        else:
            raise Exception(f"Unsupported auth_type: {auth_type}")

    async def _validate_max_body_size(security_cfg):
        """Check request size does not exceed max_body_size."""
        max_size = security_cfg.get("max_body_size")
        if not max_size:
            return

        # Convert "10MB" → bytes
        units = {"kb": 1024, "mb": 1024**2, "gb": 1024**3}
        size_str = max_size.lower()

        for suffix, factor in units.items():
            if size_str.endswith(suffix):
                limit = int(size_str.replace(suffix, "")) * factor
                break
        else:
            raise Exception("Invalid max_body_size format")

        content_length = request.content_length or 0
        if content_length > limit:
            raise Exception(f"Request body too large: {content_length} > {limit}")

    def _validate_ip_whitelist(security_cfg):
        """Allow only IPs listed in ip_whitelist."""
        whitelist = security_cfg.get("ip_whitelist", [])
        if not whitelist:
            return

        client_ip = request.remote_addr


        for rule in whitelist:
            if "/" in rule:
                # CIDR notation
                if ipaddress.ip_address(client_ip) in ipaddress.ip_network(rule, strict=False):
                    return
            else:
                # Single IP
                if client_ip == rule:
                    return

        raise Exception(f"IP {client_ip} is not allowed by whitelist")

    def _validate_rate_limit(security_cfg):
        """Simple in-memory rate limiting."""
        rl = security_cfg.get("rate_limit")
        if not rl:
            return

        limit = rl.get("limit", 60)
        per = rl.get("per", "minute")

        window = {"second": 1, "minute": 60, "hour": 3600, "day": 86400}.get(per, 60)
        key = f"rl:{agent_id}"

        now = int(time.time())
        bucket = _rate_limit_cache.get(key, {"ts": now, "count": 0})

        # Reset window
        if now - bucket["ts"] > window:
            bucket = {"ts": now, "count": 0}

        bucket["count"] += 1
        _rate_limit_cache[key] = bucket

        if bucket["count"] > limit:
            raise Exception("Too many requests (rate limit exceeded)")

    def _validate_token_auth(security_cfg):
        """Validate header-based token authentication."""
        token_cfg = security_cfg.get("token",{})
        header = token_cfg.get("token_header")
        token_value = token_cfg.get("token_value")

        provided = request.headers.get(header)
        if provided != token_value:
            raise Exception("Invalid token authentication")

    def _validate_basic_auth(security_cfg):
        """Validate HTTP Basic Auth credentials."""
        auth_cfg = security_cfg.get("basic_auth", {})
        username = auth_cfg.get("username")
        password = auth_cfg.get("password")

        auth = request.authorization
        if not auth or auth.username != username or auth.password != password:
            raise Exception("Invalid Basic Auth credentials")

    def _validate_jwt_auth(security_cfg):
        """Validate JWT token in Authorization header."""
        jwt_cfg = security_cfg.get("jwt", {})
        secret = jwt_cfg.get("secret")
        required_claims = jwt_cfg.get("required_claims", [])

        auth_header = request.headers.get("Authorization", "")
        if not auth_header.startswith("Bearer "):
            raise Exception("Missing Bearer token")

        token = auth_header[len("Bearer "):].strip()
        if not token:
            raise Exception("Empty Bearer token")

        alg = (jwt_cfg.get("algorithm") or "HS256").upper()

        decode_kwargs = {
            "key": secret,
            "algorithms": [alg],
        }
        options = {}
        if jwt_cfg.get("audience"):
            decode_kwargs["audience"] = jwt_cfg["audience"]
            options["verify_aud"] = True
        else:
            options["verify_aud"] = False

        if jwt_cfg.get("issuer"):
            decode_kwargs["issuer"] = jwt_cfg["issuer"]
            options["verify_iss"] = True
        else:
            options["verify_iss"] = False
        try:
            decoded = jwt.decode(
                token,
                options=options,
                **decode_kwargs,
            )
        except Exception as e:
            raise Exception(f"Invalid JWT: {str(e)}")

        raw_required_claims = jwt_cfg.get("required_claims", [])
        if isinstance(raw_required_claims, str):
            required_claims = [raw_required_claims]
        elif isinstance(raw_required_claims, (list, tuple, set)):
            required_claims = list(raw_required_claims)
        else:
            required_claims = []

        required_claims = [
            c for c in required_claims
            if isinstance(c, str) and c.strip()
        ]

        RESERVED_CLAIMS = {"exp", "sub", "aud", "iss", "nbf", "iat"}
        for claim in required_claims:
            if claim in RESERVED_CLAIMS:
                raise Exception(f"Reserved JWT claim cannot be required: {claim}")

        for claim in required_claims:
            if claim not in decoded:
                raise Exception(f"Missing JWT claim: {claim}")

        return decoded

    try:
        security_config=webhook_cfg.get("security", {})
        await validate_webhook_security(security_config)
    except Exception as e:
        return get_data_error_result(message=str(e))
    if not isinstance(cvs.dsl, str):
        dsl = json.dumps(cvs.dsl, ensure_ascii=False)
    try:
        canvas = Canvas(dsl, cvs.user_id, agent_id)
    except Exception as e:
        return get_json_result(
            data=False, message=str(e),
            code=RetCode.EXCEPTION_ERROR)

    # 7. Parse request body
    async def parse_webhook_request(content_type):
        """Parse request based on content-type and return structured data."""

        # 1. Query
        query_data = {k: v for k, v in request.args.items()}

        # 2. Headers
        header_data = {k: v for k, v in request.headers.items()}

        # 3. Body
        ctype = request.headers.get("Content-Type", "").split(";")[0].strip()
        if ctype != content_type:
            raise ValueError(
                f"Invalid Content-Type: expect '{content_type}', got '{ctype or 'empty'}'"
            )

        body_data: dict = {}

        try:
            if ctype == "application/json":
                body_data = await request.get_json() or {}

            elif ctype == "multipart/form-data":
                nonlocal canvas
                form = await request.form
                files = await request.files

                body_data = {}

                for key, value in form.items():
                    body_data[key] = value

                for key, file in files.items():
                    desc = FileService.upload_info(
                        cvs.user_id,           # user
                        file,              # FileStorage
                        None                   # url (None for webhook)
                    )
                    file_parsed= await canvas.get_files_async([desc])
                    body_data[key] = file_parsed

            elif ctype == "application/x-www-form-urlencoded":
                form = await request.form
                body_data = dict(form)

            else:
                # text/plain / octet-stream / empty / unknown
                raw = await request.get_data()
                if raw:
                    try:
                        body_data = json.loads(raw.decode("utf-8"))
                    except Exception:
                        body_data = {}
                else:
                    body_data = {}

        except Exception:
            body_data = {}

        return {
            "query": query_data,
            "headers": header_data,
            "body": body_data,
            "content_type": ctype,
        }

    def extract_by_schema(data, schema, name="section"):
        """
        Extract only fields defined in schema.
        Required fields must exist.
        Optional fields default to type-based default values.
        Type validation included.
        """
        props = schema.get("properties", {})
        required = schema.get("required", [])

        extracted = {}

        for field, field_schema in props.items():
            field_type = field_schema.get("type")

            # 1. Required field missing
            if field in required and field not in data:
                raise Exception(f"{name} missing required field: {field}")

            # 2. Optional → default value
            if field not in data:
                extracted[field] = default_for_type(field_type)
                continue

            raw_value = data[field]

            # 3. Auto convert value
            try:
                value = auto_cast_value(raw_value, field_type)
            except Exception as e:
                raise Exception(f"{name}.{field} auto-cast failed: {str(e)}")

            # 4. Type validation
            if not validate_type(value, field_type):
                raise Exception(
                    f"{name}.{field} type mismatch: expected {field_type}, got {type(value).__name__}"
                )

            extracted[field] = value

        return extracted


    def default_for_type(t):
        """Return default value for the given schema type."""
        if t == "file":
            return []
        if t == "object":
            return {}
        if t == "boolean":
            return False
        if t == "number":
            return 0
        if t == "string":
            return ""
        if t and t.startswith("array"):
            return []
        if t == "null":
            return None
        return None

    def auto_cast_value(value, expected_type):
        """Convert string values into schema type when possible."""

        # Non-string values already good
        if not isinstance(value, str):
            return value

        v = value.strip()

        # Boolean
        if expected_type == "boolean":
            if v.lower() in ["true", "1"]:
                return True
            if v.lower() in ["false", "0"]:
                return False
            raise Exception(f"Cannot convert '{value}' to boolean")

        # Number
        if expected_type == "number":
            # integer
            if v.isdigit() or (v.startswith("-") and v[1:].isdigit()):
                return int(v)

            # float
            try:
                return float(v)
            except Exception:
                raise Exception(f"Cannot convert '{value}' to number")

        # Object
        if expected_type == "object":
            try:
                parsed = json.loads(v)
                if isinstance(parsed, dict):
                    return parsed
                else:
                    raise Exception("JSON is not an object")
            except Exception:
                raise Exception(f"Cannot convert '{value}' to object")

        # Array <T>
        if expected_type.startswith("array"):
            try:
                parsed = json.loads(v)
                if isinstance(parsed, list):
                    return parsed
                else:
                    raise Exception("JSON is not an array")
            except Exception:
                raise Exception(f"Cannot convert '{value}' to array")

        # String (accept original)
        if expected_type == "string":
            return value

        # File
        if expected_type == "file":
            return value
        # Default: do nothing
        return value


    def validate_type(value, t):
        """Validate value type against schema type t."""
        if t == "file":
            return isinstance(value, list)

        if t == "string":
            return isinstance(value, str)

        if t == "number":
            return isinstance(value, (int, float))

        if t == "boolean":
            return isinstance(value, bool)

        if t == "object":
            return isinstance(value, dict)

        # array<string> / array<number> / array<object>
        if t.startswith("array"):
            if not isinstance(value, list):
                return False

            if "<" in t and ">" in t:
                inner = t[t.find("<") + 1 : t.find(">")]

                # Check each element type
                for item in value:
                    if not validate_type(item, inner):
                        return False

            return True

        return True

    parsed = await parse_webhook_request(webhook_cfg.get("content_types"))
    SCHEMA = webhook_cfg.get("schema", {"query": {}, "headers": {}, "body": {}})

    # Extract strictly by schema
    query_clean  = extract_by_schema(parsed["query"],   SCHEMA.get("query", {}),  name="query")
    header_clean = extract_by_schema(parsed["headers"], SCHEMA.get("headers", {}), name="headers")
    body_clean   = extract_by_schema(parsed["body"],    SCHEMA.get("body", {}),    name="body")

    clean_request = {
        "query": query_clean,
        "headers": header_clean,
        "body": body_clean
    }

    execution_mode = webhook_cfg.get("execution_mode", "Immediately")
    response_cfg = webhook_cfg.get("response", {})

    if execution_mode == "Immediately":
        status = response_cfg.get("status", 200)
        body_tpl = response_cfg.get("body_template", "")

        def parse_body(body: str):
            if not body:
                return None, "application/json"

            try:
                parsed = json.loads(body)
                return parsed, "application/json"
            except (json.JSONDecodeError, TypeError):
                return body, "text/plain"


        body, content_type = parse_body(body_tpl)
        resp = Response(
            json.dumps(body, ensure_ascii=False) if content_type == "application/json" else body,
            status=status,
            content_type=content_type,
        )

        async def background_run():
            try:
                async for _ in canvas.run(
                    query="",
                    user_id=cvs.user_id,
                    webhook_payload=clean_request
                ):
                    pass  # or log/save ans

                cvs.dsl = json.loads(str(canvas))
                UserCanvasService.update_by_id(cvs.user_id, cvs.to_dict())

            except Exception as e:
                logging.exception(f"Webhook background run failed: {e}")

        asyncio.create_task(background_run())
        return resp
    else:
        async def sse():
            nonlocal canvas
            contents: list[str] = []

            try:
                async for ans in canvas.run(
                    query="",
                    user_id=cvs.user_id,
                    webhook_payload=clean_request,
                ):
                    if ans.get("event") == "message":
                        content = (ans.get("data") or {}).get("content")
                        if content:
                            contents.append(content)

                final_content = "".join(contents)
                yield json.dumps(final_content, ensure_ascii=False)

            except Exception as e:
                yield json.dumps({"code": 500, "message": str(e)}, ensure_ascii=False)

        resp = Response(sse(), mimetype="application/json")
        return resp