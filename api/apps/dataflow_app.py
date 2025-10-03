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
import sys
import time
from functools import partial

import trio
from flask import request
from flask_login import current_user, login_required

from agent.canvas import Canvas
from agent.component.llm import LLM
from api.db import CanvasCategory, FileType
from api.db.services.canvas_service import CanvasTemplateService, UserCanvasService
from api.db.services.document_service import DocumentService
from api.db.services.file_service import FileService
from api.db.services.task_service import queue_dataflow
from api.db.services.user_canvas_version import UserCanvasVersionService
from api.db.services.user_service import TenantService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import get_data_error_result, get_json_result, server_error_response, validate_request
from api.utils.file_utils import filename_type, read_potential_broken_pdf
from rag.flow.pipeline import Pipeline


@manager.route("/templates", methods=["GET"])  # noqa: F821
@login_required
def templates():
    return get_json_result(data=[c.to_dict() for c in CanvasTemplateService.query(canvas_category=CanvasCategory.DataFlow)])


@manager.route("/list", methods=["GET"])  # noqa: F821
@login_required
def canvas_list():
    return get_json_result(data=sorted([c.to_dict() for c in UserCanvasService.query(user_id=current_user.id, canvas_category=CanvasCategory.DataFlow)], key=lambda x: x["update_time"] * -1))


@manager.route("/rm", methods=["POST"])  # noqa: F821
@validate_request("canvas_ids")
@login_required
def rm():
    for i in request.json["canvas_ids"]:
        if not UserCanvasService.accessible(i, current_user.id):
            return get_json_result(data=False, message="Only owner of canvas authorized for this operation.", code=RetCode.OPERATING_ERROR)
        UserCanvasService.delete_by_id(i)
    return get_json_result(data=True)


@manager.route("/set", methods=["POST"])  # noqa: F821
@validate_request("dsl", "title")
@login_required
def save():
    req = request.json
    if not isinstance(req["dsl"], str):
        req["dsl"] = json.dumps(req["dsl"], ensure_ascii=False)
    req["dsl"] = json.loads(req["dsl"])
    req["canvas_category"] = CanvasCategory.DataFlow
    if "id" not in req:
        req["user_id"] = current_user.id
        if UserCanvasService.query(user_id=current_user.id, title=req["title"].strip(), canvas_category=CanvasCategory.DataFlow):
            return get_data_error_result(message=f"{req['title'].strip()} already exists.")
        req["id"] = get_uuid()

        if not UserCanvasService.save(**req):
            return get_data_error_result(message="Fail to save canvas.")
    else:
        if not UserCanvasService.accessible(req["id"], current_user.id):
            return get_json_result(data=False, message="Only owner of canvas authorized for this operation.", code=RetCode.OPERATING_ERROR)
        UserCanvasService.update_by_id(req["id"], req)
    # save version
    UserCanvasVersionService.insert(user_canvas_id=req["id"], dsl=req["dsl"], title="{0}_{1}".format(req["title"], time.strftime("%Y_%m_%d_%H_%M_%S")))
    UserCanvasVersionService.delete_all_versions(req["id"])
    return get_json_result(data=req)


@manager.route("/get/<canvas_id>", methods=["GET"])  # noqa: F821
@login_required
def get(canvas_id):
    if not UserCanvasService.accessible(canvas_id, current_user.id):
        return get_data_error_result(message="canvas not found.")
    e, c = UserCanvasService.get_by_canvas_id(canvas_id)
    return get_json_result(data=c)


@manager.route("/run", methods=["POST"])  # noqa: F821
@validate_request("id")
@login_required
def run():
    req = request.json
    flow_id = req.get("id", "")
    doc_id = req.get("doc_id", "")
    if not all([flow_id, doc_id]):
        return get_data_error_result(message="id and doc_id are required.")

    if not DocumentService.get_by_id(doc_id):
        return get_data_error_result(message=f"Document for {doc_id} not found.")

    user_id = req.get("user_id", current_user.id)
    if not UserCanvasService.accessible(flow_id, current_user.id):
        return get_json_result(data=False, message="Only owner of canvas authorized for this operation.", code=RetCode.OPERATING_ERROR)

    e, cvs = UserCanvasService.get_by_id(flow_id)
    if not e:
        return get_data_error_result(message="canvas not found.")

    if not isinstance(cvs.dsl, str):
        cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)

    task_id = get_uuid()

    ok, error_message = queue_dataflow(dsl=cvs.dsl, tenant_id=user_id, doc_id=doc_id, task_id=task_id, flow_id=flow_id, priority=0)
    if not ok:
        return server_error_response(error_message)

    return get_json_result(data={"task_id": task_id, "flow_id": flow_id})


@manager.route("/reset", methods=["POST"])  # noqa: F821
@validate_request("id")
@login_required
def reset():
    req = request.json
    flow_id = req.get("id", "")
    if not flow_id:
        return get_data_error_result(message="id is required.")

    if not UserCanvasService.accessible(flow_id, current_user.id):
        return get_json_result(data=False, message="Only owner of canvas authorized for this operation.", code=RetCode.OPERATING_ERROR)

    task_id = req.get("task_id", "")

    try:
        e, user_canvas = UserCanvasService.get_by_id(req["id"])
        if not e:
            return get_data_error_result(message="canvas not found.")

        dataflow = Pipeline(dsl=json.dumps(user_canvas.dsl), tenant_id=current_user.id, flow_id=flow_id, task_id=task_id)
        dataflow.reset()
        req["dsl"] = json.loads(str(dataflow))
        UserCanvasService.update_by_id(req["id"], {"dsl": req["dsl"]})
        return get_json_result(data=req["dsl"])
    except Exception as e:
        return server_error_response(e)


@manager.route("/upload/<canvas_id>", methods=["POST"])  # noqa: F821
def upload(canvas_id):
    e, cvs = UserCanvasService.get_by_canvas_id(canvas_id)
    if not e:
        return get_data_error_result(message="canvas not found.")

    user_id = cvs["user_id"]

    def structured(filename, filetype, blob, content_type):
        nonlocal user_id
        if filetype == FileType.PDF.value:
            blob = read_potential_broken_pdf(blob)

        location = get_uuid()
        FileService.put_blob(user_id, location, blob)

        return {
            "id": location,
            "name": filename,
            "size": sys.getsizeof(blob),
            "extension": filename.split(".")[-1].lower(),
            "mime_type": content_type,
            "created_by": user_id,
            "created_at": time.time(),
            "preview_url": None,
        }

    if request.args.get("url"):
        from crawl4ai import AsyncWebCrawler, BrowserConfig, CrawlerRunConfig, CrawlResult, DefaultMarkdownGenerator, PruningContentFilter

        try:
            url = request.args.get("url")
            filename = re.sub(r"\?.*", "", url.split("/")[-1])

            async def adownload():
                browser_config = BrowserConfig(
                    headless=True,
                    verbose=False,
                )
                async with AsyncWebCrawler(config=browser_config) as crawler:
                    crawler_config = CrawlerRunConfig(markdown_generator=DefaultMarkdownGenerator(content_filter=PruningContentFilter()), pdf=True, screenshot=False)
                    result: CrawlResult = await crawler.arun(url=url, config=crawler_config)
                    return result

            page = trio.run(adownload())
            if page.pdf:
                if filename.split(".")[-1].lower() != "pdf":
                    filename += ".pdf"
                return get_json_result(data=structured(filename, "pdf", page.pdf, page.response_headers["content-type"]))

            return get_json_result(data=structured(filename, "html", str(page.markdown).encode("utf-8"), page.response_headers["content-type"], user_id))

        except Exception as e:
            return server_error_response(e)

    file = request.files["file"]
    try:
        DocumentService.check_doc_health(user_id, file.filename)
        return get_json_result(data=structured(file.filename, filename_type(file.filename), file.read(), file.content_type))
    except Exception as e:
        return server_error_response(e)


@manager.route("/input_form", methods=["GET"])  # noqa: F821
@login_required
def input_form():
    flow_id = request.args.get("id")
    cpn_id = request.args.get("component_id")
    try:
        e, user_canvas = UserCanvasService.get_by_id(flow_id)
        if not e:
            return get_data_error_result(message="canvas not found.")
        if not UserCanvasService.query(user_id=current_user.id, id=flow_id):
            return get_json_result(data=False, message="Only owner of canvas authorized for this operation.", code=RetCode.OPERATING_ERROR)

        dataflow = Pipeline(dsl=json.dumps(user_canvas.dsl), tenant_id=current_user.id, flow_id=flow_id, task_id="")

        return get_json_result(data=dataflow.get_component_input_form(cpn_id))
    except Exception as e:
        return server_error_response(e)


@manager.route("/debug", methods=["POST"])  # noqa: F821
@validate_request("id", "component_id", "params")
@login_required
def debug():
    req = request.json
    if not UserCanvasService.accessible(req["id"], current_user.id):
        return get_json_result(data=False, message="Only owner of canvas authorized for this operation.", code=RetCode.OPERATING_ERROR)
    try:
        e, user_canvas = UserCanvasService.get_by_id(req["id"])
        canvas = Canvas(json.dumps(user_canvas.dsl), current_user.id)
        canvas.reset()
        canvas.message_id = get_uuid()
        component = canvas.get_component(req["component_id"])["obj"]
        component.reset()

        if isinstance(component, LLM):
            component.set_debug_inputs(req["params"])
        component.invoke(**{k: o["value"] for k, o in req["params"].items()})
        outputs = component.output()
        for k in outputs.keys():
            if isinstance(outputs[k], partial):
                txt = ""
                for c in outputs[k]():
                    txt += c
                outputs[k] = txt
        return get_json_result(data=outputs)
    except Exception as e:
        return server_error_response(e)


# api get list version dsl of canvas
@manager.route("/getlistversion/<canvas_id>", methods=["GET"])  # noqa: F821
@login_required
def getlistversion(canvas_id):
    try:
        list = sorted([c.to_dict() for c in UserCanvasVersionService.list_by_canvas_id(canvas_id)], key=lambda x: x["update_time"] * -1)
        return get_json_result(data=list)
    except Exception as e:
        return get_data_error_result(message=f"Error getting history files: {e}")


# api get version dsl of canvas
@manager.route("/getversion/<version_id>", methods=["GET"])  # noqa: F821
@login_required
def getversion(version_id):
    try:
        e, version = UserCanvasVersionService.get_by_id(version_id)
        if version:
            return get_json_result(data=version.to_dict())
    except Exception as e:
        return get_json_result(data=f"Error getting history file: {e}")


@manager.route("/listteam", methods=["GET"])  # noqa: F821
@login_required
def list_canvas():
    keywords = request.args.get("keywords", "")
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 150))
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", True)
    try:
        tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
        canvas, total = UserCanvasService.get_by_tenant_ids(
            [m["tenant_id"] for m in tenants], current_user.id, page_number, items_per_page, orderby, desc, keywords, canvas_category=CanvasCategory.DataFlow
        )
        return get_json_result(data={"canvas": canvas, "total": total})
    except Exception as e:
        return server_error_response(e)


@manager.route("/setting", methods=["POST"])  # noqa: F821
@validate_request("id", "title", "permission")
@login_required
def setting():
    req = request.json
    req["user_id"] = current_user.id

    if not UserCanvasService.accessible(req["id"], current_user.id):
        return get_json_result(data=False, message="Only owner of canvas authorized for this operation.", code=RetCode.OPERATING_ERROR)

    e, flow = UserCanvasService.get_by_id(req["id"])
    if not e:
        return get_data_error_result(message="canvas not found.")
    flow = flow.to_dict()
    flow["title"] = req["title"]
    for key in ("description", "permission", "avatar"):
        if value := req.get(key):
            flow[key] = value

    num = UserCanvasService.update_by_id(req["id"], flow)
    return get_json_result(data=num)


@manager.route("/trace", methods=["GET"])  # noqa: F821
def trace():
    dataflow_id = request.args.get("dataflow_id")
    task_id = request.args.get("task_id")
    if not all([dataflow_id, task_id]):
        return get_data_error_result(message="dataflow_id and task_id are required.")

    e, dataflow_canvas = UserCanvasService.get_by_id(dataflow_id)
    if not e:
        return get_data_error_result(message="dataflow not found.")

    dsl_str = json.dumps(dataflow_canvas.dsl, ensure_ascii=False)
    dataflow = Pipeline(dsl=dsl_str, tenant_id=dataflow_canvas.user_id, flow_id=dataflow_id, task_id=task_id)
    log = dataflow.fetch_logs()

    return get_json_result(data=log)
