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
import logging
import re
from io import BytesIO

from flask import request
from flask_login import login_required, current_user
from typing import Any  # for IDE type checkers

from api.db.services import duplicate_name
from api.db.services.document_service import DocumentService, queue_raptor_o_graphrag_tasks
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.pipeline_operation_log_service import PipelineOperationLogService
from api.db.services.task_service import TaskService, GRAPH_RAPTOR_FAKE_DOC_ID
from api.db.services.user_service import TenantService, UserTenantService
from api.utils.api_utils import get_error_data_result, server_error_response, get_data_error_result, validate_request, not_allowed_parameters
from api.utils import get_uuid
from api.db import PipelineTaskType, StatusEnum, FileSource, VALID_FILE_TYPES, VALID_TASK_STATUS
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.search_service import SearchService
from api.db.services.dialog_service import meta_filter
from api.db.services.llm_service import LLMBundle
from api.db import LLMType
from rag.prompts.generator import gen_meta_filter, cross_languages, keyword_extraction
from rag.app.tag import label_question
from api.db.db_models import File
from api.utils.api_utils import get_json_result, send_file_in_mem
from api import settings
from rag.nlp import search
from api.constants import DATASET_NAME_LIMIT
from rag.settings import PAGERANK_FLD
from rag.utils.redis_conn import REDIS_CONN
from rag.utils.storage_factory import STORAGE_IMPL


@manager.route('/create', methods=['post'])  # noqa: F821
@login_required
@validate_request("name")
def create():
  req = request.json
  dataset_name = req["name"]
  if not isinstance(dataset_name, str):
    return get_data_error_result(message="Dataset name must be string.")
  if dataset_name.strip() == "":
    return get_data_error_result(message="Dataset name can't be empty.")
  if len(dataset_name.encode("utf-8")) > DATASET_NAME_LIMIT:
    return get_data_error_result(
        message=f"Dataset name length is {len(dataset_name)} which is larger than {DATASET_NAME_LIMIT}")

  dataset_name = dataset_name.strip()
  dataset_name = duplicate_name(
      KnowledgebaseService.query, name=dataset_name, tenant_id=current_user.id, status=StatusEnum.VALID.value)
  try:
    req["id"] = get_uuid()
    req["name"] = dataset_name
    req["tenant_id"] = current_user.id
    req["created_by"] = current_user.id
    if not req.get("parser_id"):
      req["parser_id"] = "naive"
    e, t = TenantService.get_by_id(current_user.id)
    if not e:
      return get_data_error_result(message="Tenant not found.")
    req["parser_config"] = {
        "layout_recognize": "DeepDOC",
        "chunk_token_num": 512,
        "delimiter": "\n",
        "auto_keywords": 0,
        "auto_questions": 0,
        "html4excel": False,
        "topn_tags": 3,
        "raptor": {
            "use_raptor":
                True,
            "prompt":
                "Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize.",
            "max_token":
                256,
            "threshold":
                0.1,
            "max_cluster":
                64,
            "random_seed":
                0
        },
        "graphrag": {
            "use_graphrag": True,
            "entity_types": ["organization", "person", "geo", "event", "category"],
            "method": "light"
        }
    }
    if not KnowledgebaseService.save(**req):
      return get_data_error_result()
    return get_json_result(data={"kb_id": req["id"]})
  except Exception as e:
    return server_error_response(e)


@manager.route('/update', methods=['post'])  # noqa: F821
@login_required
@validate_request("kb_id", "name", "description", "parser_id")
@not_allowed_parameters("id",
                        "tenant_id",
                        "created_by",
                        "create_time",
                        "update_time",
                        "create_date",
                        "update_date",
                        "created_by")
def update():
  req = request.json
  if not isinstance(req["name"], str):
    return get_data_error_result(message="Dataset name must be string.")
  if req["name"].strip() == "":
    return get_data_error_result(message="Dataset name can't be empty.")
  if len(req["name"].encode("utf-8")) > DATASET_NAME_LIMIT:
    return get_data_error_result(
        message=f"Dataset name length is {len(req['name'])} which is large than {DATASET_NAME_LIMIT}")
  req["name"] = req["name"].strip()

  if not KnowledgebaseService.accessible4deletion(req["kb_id"], current_user.id):
    return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)
  try:
    if not KnowledgebaseService.query(created_by=current_user.id, id=req["kb_id"]):
      return get_json_result(
          data=False,
          message='Only owner of knowledgebase authorized for this operation.',
          code=settings.RetCode.OPERATING_ERROR)

    e, kb = KnowledgebaseService.get_by_id(req["kb_id"])
    if not e:
      return get_data_error_result(message="Can't find this knowledgebase!")

    if req["name"].lower() != kb.name.lower() \
            and len(
        KnowledgebaseService.query(name=req["name"], tenant_id=current_user.id, status=StatusEnum.VALID.value)) >= 1:
      return get_data_error_result(message="Duplicated knowledgebase name.")

    del req["kb_id"]
    if not KnowledgebaseService.update_by_id(kb.id, req):
      return get_data_error_result()

    if kb.pagerank != req.get("pagerank", 0):
      if req.get("pagerank", 0) > 0:
        settings.docStoreConn.update({"kb_id": kb.id}, {PAGERANK_FLD: req["pagerank"]},
                                     search.index_name(kb.tenant_id),
                                     kb.id)
      else:
        # Elasticsearch requires PAGERANK_FLD be non-zero!
        settings.docStoreConn.update({"exists": PAGERANK_FLD}, {"remove": PAGERANK_FLD},
                                     search.index_name(kb.tenant_id),
                                     kb.id)

    e, kb = KnowledgebaseService.get_by_id(kb.id)
    if not e:
      return get_data_error_result(message="Database error (Knowledgebase rename)!")
    kb = kb.to_dict()
    kb.update(req)

    return get_json_result(data=kb)
  except Exception as e:
    return server_error_response(e)


@manager.route('/detail', methods=['GET'])  # noqa: F821
@login_required
def detail():
  kb_id = request.args["kb_id"]
  try:
    tenants = UserTenantService.query(user_id=current_user.id)
    for tenant in tenants:
      if KnowledgebaseService.query(tenant_id=tenant.tenant_id, id=kb_id):
        break
    else:
      return get_json_result(
          data=False,
          message='Only owner of knowledgebase authorized for this operation.',
          code=settings.RetCode.OPERATING_ERROR)
    kb = KnowledgebaseService.get_detail(kb_id)
    if not kb:
      return get_data_error_result(message="Can't find this knowledgebase!")
    kb["size"] = DocumentService.get_total_size_by_kb_id(kb_id=kb["id"], keywords="", run_status=[], types=[])
    for key in ["graphrag_task_finish_at", "raptor_task_finish_at", "mindmap_task_finish_at"]:
      if finish_at := kb.get(key):
        kb[key] = finish_at.strftime("%Y-%m-%d %H:%M:%S")
    return get_json_result(data=kb)
  except Exception as e:
    return server_error_response(e)


@manager.route('/list', methods=['POST'])  # noqa: F821
@login_required
def list_kbs():
  keywords = request.args.get("keywords", "")
  page_number = int(request.args.get("page", 0))
  items_per_page = int(request.args.get("page_size", 0))
  parser_id = request.args.get("parser_id")
  orderby = request.args.get("orderby", "create_time")
  if request.args.get("desc", "true").lower() == "false":
    desc = False
  else:
    desc = True

  req = request.get_json()
  owner_ids = req.get("owner_ids", [])
  try:
    if not owner_ids:
      tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
      tenants = [m["tenant_id"] for m in tenants]
      kbs, total = KnowledgebaseService.get_by_tenant_ids(
          tenants, current_user.id, page_number,
          items_per_page, orderby, desc, keywords, parser_id)
    else:
      tenants = owner_ids
      kbs, total = KnowledgebaseService.get_by_tenant_ids(
          tenants, current_user.id, 0,
          0, orderby, desc, keywords, parser_id)
      kbs = [kb for kb in kbs if kb["tenant_id"] in tenants]
      total = len(kbs)
      if page_number and items_per_page:
        kbs = kbs[(page_number - 1) * items_per_page:page_number * items_per_page]
    return get_json_result(data={"kbs": kbs, "total": total})
  except Exception as e:
    return server_error_response(e)


@manager.route('/rm', methods=['post'])  # noqa: F821
@login_required
@validate_request("kb_id")
def rm():
  req = request.json
  if not KnowledgebaseService.accessible4deletion(req["kb_id"], current_user.id):
    return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)
  try:
    kbs = KnowledgebaseService.query(created_by=current_user.id, id=req["kb_id"])
    if not kbs:
      return get_json_result(
          data=False,
          message='Only owner of knowledgebase authorized for this operation.',
          code=settings.RetCode.OPERATING_ERROR)

    for doc in DocumentService.query(kb_id=req["kb_id"]):
      if not DocumentService.remove_document(doc, kbs[0].tenant_id):
        return get_data_error_result(message="Database error (Document removal)!")
      f2d = File2DocumentService.get_by_document_id(doc.id)
      if f2d:
        FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
      File2DocumentService.delete_by_document_id(doc.id)
    FileService.filter_delete(
        [File.source_type == FileSource.KNOWLEDGEBASE, File.type == "folder", File.name == kbs[0].name])
    if not KnowledgebaseService.delete_by_id(req["kb_id"]):
      return get_data_error_result(message="Database error (Knowledgebase removal)!")
    for kb in kbs:
      settings.docStoreConn.delete({"kb_id": kb.id}, search.index_name(kb.tenant_id), kb.id)
      settings.docStoreConn.deleteIdx(search.index_name(kb.tenant_id), kb.id)
      if hasattr(STORAGE_IMPL, 'remove_bucket'):
        STORAGE_IMPL.remove_bucket(kb.id)
    return get_json_result(data=True)
  except Exception as e:
    return server_error_response(e)


@manager.route('/<kb_id>/tags', methods=['GET'])  # noqa: F821
@login_required
def list_tags(kb_id):
  if not KnowledgebaseService.accessible(kb_id, current_user.id):
    return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)

  tenants = UserTenantService.get_tenants_by_user_id(current_user.id)
  tags = []
  for tenant in tenants:
    tags += settings.retriever.all_tags(tenant["tenant_id"], [kb_id])
  return get_json_result(data=tags)


@manager.route('/tags', methods=['GET'])  # noqa: F821
@login_required
def list_tags_from_kbs():
  kb_ids = request.args.get("kb_ids", "").split(",")
  for kb_id in kb_ids:
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
      return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)

  tenants = UserTenantService.get_tenants_by_user_id(current_user.id)
  tags = []
  for tenant in tenants:
    tags += settings.retriever.all_tags(tenant["tenant_id"], kb_ids)
  return get_json_result(data=tags)


@manager.route('/<kb_id>/rm_tags', methods=['POST'])  # noqa: F821
@login_required
def rm_tags(kb_id):
  req = request.json
  if not KnowledgebaseService.accessible(kb_id, current_user.id):
    return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)
  e, kb = KnowledgebaseService.get_by_id(kb_id)

  for t in req["tags"]:
    settings.docStoreConn.update({
        "tag_kwd": t, "kb_id": [kb_id]
    }, {"remove": {
        "tag_kwd": t
    }},
                                 search.index_name(kb.tenant_id),
                                 kb_id)
  return get_json_result(data=True)


@manager.route('/<kb_id>/rename_tag', methods=['POST'])  # noqa: F821
@login_required
def rename_tags(kb_id):
  req = request.json
  if not KnowledgebaseService.accessible(kb_id, current_user.id):
    return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)
  e, kb = KnowledgebaseService.get_by_id(kb_id)

  settings.docStoreConn.update({
      "tag_kwd": req["from_tag"], "kb_id": [kb_id]
  }, {
      "remove": {
          "tag_kwd": req["from_tag"].strip()
      }, "add": {
          "tag_kwd": req["to_tag"]
      }
  },
                               search.index_name(kb.tenant_id),
                               kb_id)
  return get_json_result(data=True)


@manager.route('/<kb_id>/knowledge_graph', methods=['GET'])  # noqa: F821
@login_required
def knowledge_graph(kb_id):
  if not KnowledgebaseService.accessible(kb_id, current_user.id):
    return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)
  _, kb = KnowledgebaseService.get_by_id(kb_id)
  req = {"kb_id": [kb_id], "knowledge_graph_kwd": ["graph"]}

  obj = {"graph": {}, "mind_map": {}}
  if not settings.docStoreConn.indexExist(search.index_name(kb.tenant_id), kb_id):
    return get_json_result(data=obj)
  sres = settings.retriever.search(req, search.index_name(kb.tenant_id), [kb_id])
  if not len(sres.ids):
    return get_json_result(data=obj)

  for id in sres.ids[:1]:
    ty = sres.field[id]["knowledge_graph_kwd"]
    try:
      content_json = json.loads(sres.field[id]["content_with_weight"])
    except Exception:
      continue

    obj[ty] = content_json

  if "nodes" in obj["graph"]:
    obj["graph"]["nodes"] = sorted(obj["graph"]["nodes"], key=lambda x: x.get("pagerank", 0), reverse=True)[:256]
    if "edges" in obj["graph"]:
      node_id_set = {o["id"] for o in obj["graph"]["nodes"]}
      filtered_edges = [
          o for o in obj["graph"]["edges"]
          if o["source"] != o["target"] and o["source"] in node_id_set and o["target"] in node_id_set
      ]
      obj["graph"]["edges"] = sorted(filtered_edges, key=lambda x: x.get("weight", 0), reverse=True)[:128]
  return get_json_result(data=obj)


@manager.route('/<kb_id>/knowledge_graph', methods=['DELETE'])  # noqa: F821
@login_required
def delete_knowledge_graph(kb_id):
  if not KnowledgebaseService.accessible(kb_id, current_user.id):
    return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)
  _, kb = KnowledgebaseService.get_by_id(kb_id)
  settings.docStoreConn.delete({"knowledge_graph_kwd": ["graph", "subgraph", "entity", "relation"]},
                               search.index_name(kb.tenant_id),
                               kb_id)

  return get_json_result(data=True)


@manager.route("/get_meta", methods=["GET"])  # noqa: F821
@login_required
def get_meta():
  kb_ids = request.args.get("kb_ids", "").split(",")
  for kb_id in kb_ids:
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
      return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)
  return get_json_result(data=DocumentService.get_meta_by_kbs(kb_ids))


@manager.route("/basic_info", methods=["GET"])  # noqa: F821
@login_required
def get_basic_info():
  kb_id = request.args.get("kb_id", "")
  if not KnowledgebaseService.accessible(kb_id, current_user.id):
    return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)

  basic_info = DocumentService.knowledgebase_basic_info(kb_id)

  return get_json_result(data=basic_info)


@manager.route("/list_pipeline_logs", methods=["POST"])  # noqa: F821
@login_required
def list_pipeline_logs():
  kb_id = request.args.get("kb_id")
  if not kb_id:
    return get_json_result(data=False, message='Lack of "KB ID"', code=settings.RetCode.ARGUMENT_ERROR)

  keywords = request.args.get("keywords", "")

  page_number = int(request.args.get("page", 0))
  items_per_page = int(request.args.get("page_size", 0))
  orderby = request.args.get("orderby", "create_time")
  if request.args.get("desc", "true").lower() == "false":
    desc = False
  else:
    desc = True
  create_date_from = request.args.get("create_date_from", "")
  create_date_to = request.args.get("create_date_to", "")
  if create_date_to > create_date_from:
    return get_data_error_result(message="Create data filter is abnormal.")

  req = request.get_json()

  operation_status = req.get("operation_status", [])
  if operation_status:
    invalid_status = {s for s in operation_status if s not in VALID_TASK_STATUS}
    if invalid_status:
      return get_data_error_result(
          message=f"Invalid filter operation_status status conditions: {', '.join(invalid_status)}")

  types = req.get("types", [])
  if types:
    invalid_types = {t for t in types if t not in VALID_FILE_TYPES}
    if invalid_types:
      return get_data_error_result(
          message=f"Invalid filter conditions: {', '.join(invalid_types)} type{'s' if len(invalid_types) > 1 else ''}")

  suffix = req.get("suffix", [])

  try:
    logs, tol = PipelineOperationLogService.get_file_logs_by_kb_id(kb_id, page_number, items_per_page, orderby, desc, keywords, operation_status, types, suffix, create_date_from, create_date_to)
    return get_json_result(data={"total": tol, "logs": logs})
  except Exception as e:
    return server_error_response(e)


@manager.route("/list_pipeline_dataset_logs", methods=["POST"])  # noqa: F821
@login_required
def list_pipeline_dataset_logs():
  kb_id = request.args.get("kb_id")
  if not kb_id:
    return get_json_result(data=False, message='Lack of "KB ID"', code=settings.RetCode.ARGUMENT_ERROR)

  page_number = int(request.args.get("page", 0))
  items_per_page = int(request.args.get("page_size", 0))
  orderby = request.args.get("orderby", "create_time")
  if request.args.get("desc", "true").lower() == "false":
    desc = False
  else:
    desc = True
  create_date_from = request.args.get("create_date_from", "")
  create_date_to = request.args.get("create_date_to", "")
  if create_date_to > create_date_from:
    return get_data_error_result(message="Create data filter is abnormal.")

  req = request.get_json()

  operation_status = req.get("operation_status", [])
  if operation_status:
    invalid_status = {s for s in operation_status if s not in VALID_TASK_STATUS}
    if invalid_status:
      return get_data_error_result(
          message=f"Invalid filter operation_status status conditions: {', '.join(invalid_status)}")

  try:
    logs, tol = PipelineOperationLogService.get_dataset_logs_by_kb_id(kb_id, page_number, items_per_page, orderby, desc, operation_status, create_date_from, create_date_to)
    return get_json_result(data={"total": tol, "logs": logs})
  except Exception as e:
    return server_error_response(e)


@manager.route("/delete_pipeline_logs", methods=["POST"])  # noqa: F821
@login_required
def delete_pipeline_logs():
  kb_id = request.args.get("kb_id")
  if not kb_id:
    return get_json_result(data=False, message='Lack of "KB ID"', code=settings.RetCode.ARGUMENT_ERROR)

  req = request.get_json()
  log_ids = req.get("log_ids", [])

  PipelineOperationLogService.delete_by_ids(log_ids)

  return get_json_result(data=True)


@manager.route("/pipeline_log_detail", methods=["GET"])  # noqa: F821
@login_required
def pipeline_log_detail():
  log_id = request.args.get("log_id")
  if not log_id:
    return get_json_result(data=False, message='Lack of "Pipeline log ID"', code=settings.RetCode.ARGUMENT_ERROR)

  ok, log = PipelineOperationLogService.get_by_id(log_id)
  if not ok:
    return get_data_error_result(message="Invalid pipeline log ID")

  return get_json_result(data=log.to_dict())


@manager.route("/run_graphrag", methods=["POST"])  # noqa: F821
@login_required
def run_graphrag():
  req = request.json

  kb_id = req.get("kb_id", "")
  if not kb_id:
    return get_error_data_result(message='Lack of "KB ID"')

  ok, kb = KnowledgebaseService.get_by_id(kb_id)
  if not ok:
    return get_error_data_result(message="Invalid Knowledgebase ID")

  task_id = kb.graphrag_task_id
  if task_id:
    ok, task = TaskService.get_by_id(task_id)
    if not ok:
      logging.warning(f"A valid GraphRAG task id is expected for kb {kb_id}")

    if task and task.progress not in [-1, 1]:
      return get_error_data_result(
          message=f"Task {task_id} in progress with status {task.progress}. A Graph Task is already running.")

  documents, _ = DocumentService.get_by_kb_id(
      kb_id=kb_id,
      page_number=0,
      items_per_page=0,
      orderby="create_time",
      desc=False,
      keywords="",
      run_status=[],
      types=[],
      suffix=[],
  )
  if not documents:
    return get_error_data_result(message=f"No documents in Knowledgebase {kb_id}")

  sample_document = documents[0]
  document_ids = [document["id"] for document in documents]

  task_id = queue_raptor_o_graphrag_tasks(
      doc=sample_document, ty="graphrag", priority=0, fake_doc_id=GRAPH_RAPTOR_FAKE_DOC_ID, doc_ids=list(document_ids))

  if not KnowledgebaseService.update_by_id(kb.id, {"graphrag_task_id": task_id}):
    logging.warning(f"Cannot save graphrag_task_id for kb {kb_id}")

  return get_json_result(data={"graphrag_task_id": task_id})


@manager.route("/trace_graphrag", methods=["GET"])  # noqa: F821
@login_required
def trace_graphrag():
  kb_id = request.args.get("kb_id", "")
  if not kb_id:
    return get_error_data_result(message='Lack of "KB ID"')

  ok, kb = KnowledgebaseService.get_by_id(kb_id)
  if not ok:
    return get_error_data_result(message="Invalid Knowledgebase ID")

  task_id = kb.graphrag_task_id
  if not task_id:
    return get_json_result(data={})

  ok, task = TaskService.get_by_id(task_id)
  if not ok:
    return get_error_data_result(message="GraphRAG Task Not Found or Error Occurred")

  return get_json_result(data=task.to_dict())


@manager.route("/run_raptor", methods=["POST"])  # noqa: F821
@login_required
def run_raptor():
  req = request.json

  kb_id = req.get("kb_id", "")
  if not kb_id:
    return get_error_data_result(message='Lack of "KB ID"')

  ok, kb = KnowledgebaseService.get_by_id(kb_id)
  if not ok:
    return get_error_data_result(message="Invalid Knowledgebase ID")

  task_id = kb.raptor_task_id
  if task_id:
    ok, task = TaskService.get_by_id(task_id)
    if not ok:
      logging.warning(f"A valid RAPTOR task id is expected for kb {kb_id}")

    if task and task.progress not in [-1, 1]:
      return get_error_data_result(
          message=f"Task {task_id} in progress with status {task.progress}. A RAPTOR Task is already running.")

  documents, _ = DocumentService.get_by_kb_id(
      kb_id=kb_id,
      page_number=0,
      items_per_page=0,
      orderby="create_time",
      desc=False,
      keywords="",
      run_status=[],
      types=[],
      suffix=[],
  )
  if not documents:
    return get_error_data_result(message=f"No documents in Knowledgebase {kb_id}")

  sample_document = documents[0]
  document_ids = [document["id"] for document in documents]

  task_id = queue_raptor_o_graphrag_tasks(
      doc=sample_document, ty="raptor", priority=0, fake_doc_id=GRAPH_RAPTOR_FAKE_DOC_ID, doc_ids=list(document_ids))

  if not KnowledgebaseService.update_by_id(kb.id, {"raptor_task_id": task_id}):
    logging.warning(f"Cannot save raptor_task_id for kb {kb_id}")

  return get_json_result(data={"raptor_task_id": task_id})


@manager.route("/trace_raptor", methods=["GET"])  # noqa: F821
@login_required
def trace_raptor():
  kb_id = request.args.get("kb_id", "")
  if not kb_id:
    return get_error_data_result(message='Lack of "KB ID"')

  ok, kb = KnowledgebaseService.get_by_id(kb_id)
  if not ok:
    return get_error_data_result(message="Invalid Knowledgebase ID")

  task_id = kb.raptor_task_id
  if not task_id:
    return get_json_result(data={})

  ok, task = TaskService.get_by_id(task_id)
  if not ok:
    return get_error_data_result(message="RAPTOR Task Not Found or Error Occurred")

  return get_json_result(data=task.to_dict())


@manager.route("/run_mindmap", methods=["POST"])  # noqa: F821
@login_required
def run_mindmap():
  req = request.json

  kb_id = req.get("kb_id", "")
  if not kb_id:
    return get_error_data_result(message='Lack of "KB ID"')

  ok, kb = KnowledgebaseService.get_by_id(kb_id)
  if not ok:
    return get_error_data_result(message="Invalid Knowledgebase ID")

  task_id = kb.mindmap_task_id
  if task_id:
    ok, task = TaskService.get_by_id(task_id)
    if not ok:
      logging.warning(f"A valid Mindmap task id is expected for kb {kb_id}")

    if task and task.progress not in [-1, 1]:
      return get_error_data_result(
          message=f"Task {task_id} in progress with status {task.progress}. A Mindmap Task is already running.")

  documents, _ = DocumentService.get_by_kb_id(
      kb_id=kb_id,
      page_number=0,
      items_per_page=0,
      orderby="create_time",
      desc=False,
      keywords="",
      run_status=[],
      types=[],
      suffix=[],
  )
  if not documents:
    return get_error_data_result(message=f"No documents in Knowledgebase {kb_id}")

  sample_document = documents[0]
  document_ids = [document["id"] for document in documents]

  task_id = queue_raptor_o_graphrag_tasks(
      doc=sample_document, ty="mindmap", priority=0, fake_doc_id=GRAPH_RAPTOR_FAKE_DOC_ID, doc_ids=list(document_ids))

  if not KnowledgebaseService.update_by_id(kb.id, {"mindmap_task_id": task_id}):
    logging.warning(f"Cannot save mindmap_task_id for kb {kb_id}")

  return get_json_result(data={"mindmap_task_id": task_id})


@manager.route("/trace_mindmap", methods=["GET"])  # noqa: F821
@login_required
def trace_mindmap():
  kb_id = request.args.get("kb_id", "")
  if not kb_id:
    return get_error_data_result(message='Lack of "KB ID"')

  ok, kb = KnowledgebaseService.get_by_id(kb_id)
  if not ok:
    return get_error_data_result(message="Invalid Knowledgebase ID")

  task_id = kb.mindmap_task_id
  if not task_id:
    return get_json_result(data={})

  ok, task = TaskService.get_by_id(task_id)
  if not ok:
    return get_error_data_result(message="Mindmap Task Not Found or Error Occurred")

  return get_json_result(data=task.to_dict())


@manager.route("/unbind_task", methods=["DELETE"])  # noqa: F821
@login_required
def delete_kb_task():
  kb_id = request.args.get("kb_id", "")
  if not kb_id:
    return get_error_data_result(message='Lack of "KB ID"')
  ok, kb = KnowledgebaseService.get_by_id(kb_id)
  if not ok:
    return get_json_result(data=True)

  pipeline_task_type = request.args.get("pipeline_task_type", "")
  if not pipeline_task_type or pipeline_task_type not in [
      PipelineTaskType.GRAPH_RAG, PipelineTaskType.RAPTOR, PipelineTaskType.MINDMAP
  ]:
    return get_error_data_result(message="Invalid task type")

  match pipeline_task_type:
    case PipelineTaskType.GRAPH_RAG:
      settings.docStoreConn.delete({"knowledge_graph_kwd": ["graph", "subgraph", "entity", "relation"]},
                                   search.index_name(kb.tenant_id),
                                   kb_id)
      kb_task_id_field = "graphrag_task_id"
      task_id = kb.graphrag_task_id
      kb_task_finish_at = "graphrag_task_finish_at"
    case PipelineTaskType.RAPTOR:
      kb_task_id_field = "raptor_task_id"
      task_id = kb.raptor_task_id
      kb_task_finish_at = "raptor_task_finish_at"
    case PipelineTaskType.MINDMAP:
      kb_task_id_field = "mindmap_task_id"
      task_id = kb.mindmap_task_id
      kb_task_finish_at = "mindmap_task_finish_at"
    case _:
      return get_error_data_result(message="Internal Error: Invalid task type")

  def cancel_task(task_id):
    REDIS_CONN.set(f"{task_id}-cancel", "x")

  cancel_task(task_id)

  ok = KnowledgebaseService.update_by_id(kb_id, {kb_task_id_field: "", kb_task_finish_at: None})
  if not ok:
    return server_error_response(f"Internal error: cannot delete task {pipeline_task_type}")

  return get_json_result(data=True)


@manager.route("/risk_identify", methods=["POST"])  # noqa: F821
@login_required
def risk_identify():
  try:
    # Accept both knowledge_base_id and kb_id for flexibility
    kb_id = request.form.get("knowledge_base_id") or request.form.get("kb_id")
    if not kb_id:
      return get_json_result(data=False, message='Lack of "KB ID"', code=settings.RetCode.ARGUMENT_ERROR)

    # Permission check consistent with other kb endpoints
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
      return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)

    # File presence and type check
    if "data" not in request.files:
      return get_json_result(data=False, message='No file part!', code=settings.RetCode.ARGUMENT_ERROR)

    file_obj = request.files.get("data")
    if not file_obj or not getattr(file_obj, "filename", ""):
      return get_json_result(data=False, message='No file selected!', code=settings.RetCode.ARGUMENT_ERROR)

    filename = file_obj.filename
    if not filename.lower().endswith(".xlsx"):
      return get_json_result(
          data=False, message='Only .xlsx files are supported.', code=settings.RetCode.ARGUMENT_ERROR)

    # Read with pandas for concise processing
    blob = file_obj.read()
    try:
      import pandas as pd
      df = pd.read_excel(BytesIO(blob), sheet_name=0, engine="openpyxl")
    except Exception as e:
      logging.exception("Failed to load workbook with pandas for risk_identify")
      return get_json_result(data=False, message=f'Invalid xlsx file: {e}', code=settings.RetCode.DATA_ERROR)

    # Persist the original file to object storage for future use
    storage_bucket = kb_id
    base_filename = filename.rsplit('/', 1)[-1].rsplit('\\', 1)[-1]
    storage_location = f"risk_identify/{get_uuid()}_{base_filename}"
    stored_ok = False
    try:
      STORAGE_IMPL.put(storage_bucket, storage_location, blob)
      stored_ok = True
    except Exception:
      logging.exception("Failed to persist risk_identify upload to storage")

    # Normalize headers and alias mapping
    def norm_header(s: str) -> str:
      s = str(s or "").strip()
      for ch in [" ", "\u3000", ":", "：", "-", "_", "的"]:
        s = s.replace(ch, "")
      return s

    # alias set uses normalized forms for robust matching
    alias_groups = {
        "循环": {"循环"},
        "主要风险点": {"主要风险点", "风险点", "主要风险"},
        "相应的内部控制": {"相应的内部控制", "相应内部控制", "对应内部控制", "相关内部控制", "内部控制"},
    }
    col_map = {}
    for raw in df.columns:
      nh = norm_header(raw)
      for canonical, aliases in alias_groups.items():
        if nh in aliases and canonical not in col_map:
          col_map[canonical] = raw
          break

    # Build dataframe ensuring all required columns are present (fill missing with None)
    required = ["循环", "主要风险点", "相应的内部控制"]
    import pandas as pd  # ensure pd available
    df_new = pd.DataFrame()
    for key in required:
      if key in col_map:
        df_new[key] = df[col_map[key]]
      else:
        # create empty column if missing
        df_new[key] = None
    df = df_new

    # forward fill cycle and filter empty rows
    if "循环" in df.columns:
      df["循环"] = df["循环"].ffill()

    # drop rows where both risk and control are empty (if present)
    subsets = [c for c in ["主要风险点", "相应的内部控制"] if c in df.columns]
    if subsets:
      df = df.dropna(how="all", subset=subsets)

    # Convert to records with NaN -> None, strip strings
    rows_data = []
    for row in df.to_dict(orient="records"):
      rec = {}
      for k, v in row.items():
        if pd.isna(v):
          rec[k] = None
        elif isinstance(v, str):
          rec[k] = v.strip()
        else:
          rec[k] = v
      rows_data.append(rec)

    return get_json_result(
        data={
            "filename": filename,
            "headers": ["循环", "主要风险点", "相应的内部控制"],
            "rows": rows_data,
            "storage": {
                "bucket": storage_bucket,
                "location": storage_location,
                "stored": stored_ok,
            },
        })
  except Exception as e:
    return server_error_response(e)


@manager.route("/risk_retrieval", methods=["POST"])  # noqa: F821
@login_required
@validate_request("kb_id", "question")
def risk_retrieval():
  try:
    req = request.json
    page = int(req.get("page", 1))
    size = int(req.get("size", 30))
    question = req["question"]
    kb_ids = req["kb_id"]
    if isinstance(kb_ids, str):
      kb_ids = [kb_ids]
    if not kb_ids:
      return get_json_result(data=False, message='Please specify dataset firstly.', code=settings.RetCode.DATA_ERROR)

    doc_ids = req.get("doc_ids", [])
    use_kg = req.get("use_kg", False)
    top = int(req.get("top_k", 1024))
    langs = req.get("cross_languages", [])
    tenant_ids = []

    if req.get("search_id", ""):
      search_config = SearchService.get_detail(req.get("search_id", "")).get("search_config", {})
      meta_data_filter = search_config.get("meta_data_filter", {})
      metas = DocumentService.get_meta_by_kbs(kb_ids)
      if meta_data_filter.get("method") == "auto":
        chat_mdl = LLMBundle(current_user.id, LLMType.CHAT, llm_name=search_config.get("chat_id", ""))
        filters = gen_meta_filter(chat_mdl, metas, question)
        doc_ids.extend(meta_filter(metas, filters))
        if not doc_ids:
          doc_ids = None
      elif meta_data_filter.get("method") == "manual":
        doc_ids.extend(meta_filter(metas, meta_data_filter.get("manual", {})))
        if not doc_ids:
          doc_ids = None

    tenants = UserTenantService.query(user_id=current_user.id)
    for kid in kb_ids:
      for tenant in tenants:
        if KnowledgebaseService.query(tenant_id=tenant.tenant_id, id=kid):
          tenant_ids.append(tenant.tenant_id)
          break
      else:
        return get_json_result(
            data=False,
            message='Only owner of knowledgebase authorized for this operation.',
            code=settings.RetCode.OPERATING_ERROR)

    ok, kb = KnowledgebaseService.get_by_id(kb_ids[0])
    if not ok:
      return get_data_error_result(message="Knowledgebase not found!")

    if langs:
      question = cross_languages(kb.tenant_id, None, question, langs)

    embd_mdl = LLMBundle(kb.tenant_id, LLMType.EMBEDDING.value, llm_name=kb.embd_id)

    rerank_mdl = None
    if req.get("rerank_id"):
      rerank_mdl = LLMBundle(kb.tenant_id, LLMType.RERANK.value, llm_name=req["rerank_id"])

    if req.get("keyword", False):
      chat_mdl = LLMBundle(kb.tenant_id, LLMType.CHAT)
      question += keyword_extraction(chat_mdl, question)

    labels = label_question(question, [kb])
    ranks = settings.retriever.retrieval(
        question,
        embd_mdl,
        tenant_ids,
        kb_ids,
        page,
        size,
        float(req.get("similarity_threshold", 0.0)),
        float(req.get("vector_similarity_weight", 0.3)),
        top,
        doc_ids,
        rerank_mdl=rerank_mdl,
        highlight=req.get("highlight", False),
        rank_feature=labels)
    if use_kg:
      ck = settings.kg_retriever.retrieval(question,
                                           tenant_ids,
                                           kb_ids,
                                           embd_mdl,
                                           LLMBundle(kb.tenant_id, LLMType.CHAT))
      if ck.get("content_with_weight"):
        ranks["chunks"].insert(0, ck)

    for c in ranks.get("chunks", []):
      c.pop("vector", None)
    ranks["labels"] = labels

    return get_json_result(data=ranks)
  except Exception as e:
    if str(e).find("not_found") > 0:
      return get_json_result(
          data=False, message='No chunk found! Check the chunk status please!', code=settings.RetCode.DATA_ERROR)
    return server_error_response(e)


@manager.route("/risk_ai_identify", methods=["POST"])  # noqa: F821
@login_required
@validate_request("kb_id", "prompt")
def risk_ai_identify():
  """
  Use tenant's default chat model (e.g., Qwen) to run the audit instruction prompt
  and return the model output. Frontend assembles the full prompt.
  """
  try:
    req = request.json
    kb_id = req.get("kb_id", "")
    prompt = req.get("prompt", "")
    lang = req.get("lang", "Chinese")
    if not kb_id or not prompt:
      return get_json_result(data=False, message='Missing kb_id or prompt', code=settings.RetCode.ARGUMENT_ERROR)

    ok, kb = KnowledgebaseService.get_by_id(kb_id)
    if not ok:
      return get_data_error_result(message="Knowledgebase not found!")

    # Permission check (consistent with other kb endpoints)
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
      return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)

    # Build default chat model for tenant; fallback to system config if none
    chat_mdl = LLMBundle(kb.tenant_id, LLMType.CHAT, lang=lang)

    # Run model: use prompt as user content, keep system empty so frontend controls full prompt
    history = [{"role": "user", "content": str(prompt)}]
    gen_conf = {"temperature": float(req.get("temperature", 0.2))}
    answer = chat_mdl.chat("", history, gen_conf)
    # Clean fenced code like ```json ... ``` to plain content
    try:
      if isinstance(answer, str):
        m = re.search(r"```(?:json|JSON)?\s*([\s\S]*?)```", answer.strip())
        if m:
          answer = m.group(1).strip()
    except Exception:
      pass

    return get_json_result(data={"answer": answer})
  except Exception as e:
    return server_error_response(e)


@manager.route("/risk_ai_identify_batch", methods=["POST"])  # noqa: F821
@login_required
@validate_request("kb_id", "rows")
def risk_ai_identify_batch():
  try:
    req = request.json
    kb_id = req.get("kb_id", "")
    rows = req.get("rows", [])
    if not isinstance(rows, list) or not kb_id:
      return get_json_result(data=False, message='Invalid kb_id or rows', code=settings.RetCode.ARGUMENT_ERROR)

    if not KnowledgebaseService.accessible(kb_id, current_user.id):
      return get_json_result(data=False, message='No authorization.', code=settings.RetCode.AUTHENTICATION_ERROR)

    ok, kb = KnowledgebaseService.get_by_id(kb_id)
    if not ok:
      return get_data_error_result(message="Knowledgebase not found!")

    embd_mdl = LLMBundle(kb.tenant_id, LLMType.EMBEDDING.value, llm_name=kb.embd_id)
    chat_mdl = LLMBundle(kb.tenant_id, LLMType.CHAT)

    default_st = float(req.get("similarity_threshold", 0.6))
    default_vw = float(req.get("vector_similarity_weight", 0.95))
    page = int(req.get("page", 1))
    size = int(req.get("size", 10))
    parser_type = req.get("parser_type", "raw").lower()

    tenant_ids = []
    tenants = UserTenantService.query(user_id=current_user.id)
    for tenant in tenants:
      if KnowledgebaseService.query(tenant_id=tenant.tenant_id, id=kb_id):
        tenant_ids.append(tenant.tenant_id)
        break
    if not tenant_ids:
      return get_json_result(
          data=False,
          message='Only owner of knowledgebase authorized for this operation.',
          code=settings.RetCode.OPERATING_ERROR)

    from openpyxl import Workbook
    from io import BytesIO
    wb = Workbook()
    ws = wb.active
    ws.title = "AI识别结果"
    structured_headers = ["相关制度", "控制活动描述", "控制频率", "相关单据", "相关人员", "设计缺陷"]
    ws.append(["循环", "主要风险点", "相应的内部控制", *structured_headers])

    def clean_answer(txt: str) -> str:
      if not isinstance(txt, str):
        return str(txt)
      m = re.search(r"```(?:json|JSON)?\s*([\s\S]*?)```", txt.strip())
      return (m.group(1).strip() if m else txt.strip())

    for r in rows:
      cycle = (r.get("循环") or r.get("cycle") or "").strip()
      risk = (r.get("主要风险点") or r.get("risk") or "").strip()
      control = (r.get("相应的内部控制") or r.get("question") or r.get("control") or "").strip()
      if not control:
        ws.append([cycle, risk, control, *(["" for _ in structured_headers])])
        continue

      st = float(r.get("similarity_threshold", default_st))
      vw = float(r.get("vector_similarity_weight", default_vw))

      ranks = settings.retriever.retrieval(
          control,
          embd_mdl,
          tenant_ids,
          [kb_id],
          page,
          size,
          st,
          vw,
          int(req.get("top_k", 1024)),
          r.get("doc_ids"),
          rerank_mdl=None,
          highlight=False,
          rank_feature=label_question(control, [kb]),
      )
      top_chunks = ranks.get("chunks", [])
      txt = "\n\n".join([c.get("content_with_weight", "") for c in top_chunks])

      prompt = "\n".join([
          "## 角色",
          "你是一名经验丰富的内控审计专家",
          "## 任务",
          f"针对{cycle},为了防范{risk}，对“{control}”关键控制点进行审计。",
          "### 任务1",
          "根据RAG检索出的被审计单位相关内控制度:",
          f"```{txt}```",
          "筛选出与该循环的关键控制点相关的内控制度,包含制度名称和对应原文，输出“相关制度”，不相关的制度需要排除。",
          "### 任务2",
          "根据任务1识别的相关制度，整理输出“控制活动描述”，即用一句话进行专业描述，表达要客观清晰、具备可测试性，并包含控制的目的、执行人、控制内容、频率、控制方式和留痕依据等要素，但不需要逐项分点列出，只输出一条完整规范的审计用语。",
          "### 任务3",
          "输出“控制频率”，即每年一次、每季一次、每月一次、每周一次、每日一次、每日多次。如果制度中未明确则写“待填写”。",
          "### 任务4",
          "输出“相关单据”，列出控制活动中的依据或记录，如盘点表，发货单，对账单等，单据名称用书名号包裹，如：《客户信息表》。",
          "### 任务5",
          "输出“相关人员”，列出控制活动中涉及发起、执行、审核、审批等相关人员。",
          "### 任务6",
          "输出“设计缺陷”，判断是否存在内控制度设计缺陷。如果存在设计缺陷，则输出存在设计缺陷，并简要列示缺陷名称和原因。如果不存在，则输出不存在设计缺陷。",
          "## 输出",
          "将任务中需要输出的字段以json格式输出，包含：“相关制度”，“控制活动描述”，“控制频率”，“相关单据”，“相关人员”，“设计缺陷”,均为文本格式，不要是数组。",
      ])
      ans = chat_mdl.chat("", [{
          "role": "user", "content": prompt
      }], {"temperature": float(req.get("temperature", 0.2))})
      cleaned = clean_answer(ans)
      row_values = [cycle, risk, control]
      if parser_type == "structured":
        parsed_values = ["" for _ in structured_headers]
        try:
          parsed_json = json.loads(cleaned)
          for idx, key in enumerate(structured_headers):
            parsed_values[idx] = str(parsed_json.get(key, "") or "")
        except Exception:
          parsed_values[-1] = cleaned
        row_values.extend(parsed_values)
      else:
        row_values.extend(["", "", "", "", "", cleaned])

      ws.append(row_values)

    buf = BytesIO()
    wb.save(buf)
    buf.seek(0)
    filename = "控制矩阵.xlsx"
    return send_file_in_mem(buf.getvalue(), filename)
  except Exception as e:
    return server_error_response(e)
