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

"""
Deprecated, todo delete 
@manager.route('/create', methods=['post'])  # noqa: F821
@login_required
@validate_request("name")
async def create():
    req = await get_request_json()
    create_dict = ensure_tenant_model_id_for_params(current_user.id, req)
    e, res = KnowledgebaseService.create_with_name(
        name = create_dict.pop("name", None),
        tenant_id = current_user.id,
        parser_id = create_dict.pop("parser_id", None),
        **create_dict
    )

    if not e:
        return res

    try:
        if not KnowledgebaseService.save(**res):
            return get_data_error_result()
        return get_json_result(data={"kb_id":res["id"]})
    except Exception as e:
        return server_error_response(e)


@manager.route('/update', methods=['post'])  # noqa: F821
@login_required
@validate_request("kb_id", "name", "description", "parser_id")
@not_allowed_parameters("id", "tenant_id", "created_by", "create_time", "update_time", "create_date", "update_date", "created_by")
async def update():
    req = await get_request_json()
    update_dict = ensure_tenant_model_id_for_params(current_user.id, req)
    if not isinstance(update_dict["name"], str):
        return get_data_error_result(message="Dataset name must be string.")
    if update_dict["name"].strip() == "":
        return get_data_error_result(message="Dataset name can't be empty.")
    if len(update_dict["name"].encode("utf-8")) > DATASET_NAME_LIMIT:
        return get_data_error_result(
            message=f"Dataset name length is {len(update_dict['name'])} which is large than {DATASET_NAME_LIMIT}")
    update_dict["name"] = update_dict["name"].strip()
    if settings.DOC_ENGINE_INFINITY:
        parser_id = update_dict.get("parser_id")
        if isinstance(parser_id, str) and parser_id.lower() == "tag":
            return get_json_result(
                code=RetCode.OPERATING_ERROR,
                message="The chunking method Tag has not been supported by Infinity yet.",
                data=False,
            )
        if "pagerank" in update_dict and update_dict["pagerank"] > 0:
            return get_json_result(
                code=RetCode.DATA_ERROR,
                message="'pagerank' can only be set when doc_engine is elasticsearch",
                data=False,
            )

    if not KnowledgebaseService.accessible4deletion(update_dict["kb_id"], current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR
        )
    try:
        if not KnowledgebaseService.query(
                created_by=current_user.id, id=update_dict["kb_id"]):
            return get_json_result(
                data=False, message='Only owner of dataset authorized for this operation.',
                code=RetCode.OPERATING_ERROR)

        e, kb = KnowledgebaseService.get_by_id(update_dict["kb_id"])

        # Rename folder in FileService
        if e and update_dict["name"].lower() != kb.name.lower():
            FileService.filter_update(
                [
                    File.tenant_id == kb.tenant_id,
                    File.source_type == FileSource.KNOWLEDGEBASE,
                    File.type == "folder",
                    File.name == kb.name,
                ],
                {"name": update_dict["name"]},
            )

        if not e:
            return get_data_error_result(
                message="Can't find this dataset!")

        if update_dict["name"].lower() != kb.name.lower() \
                and len(
            KnowledgebaseService.query(name=update_dict["name"], tenant_id=current_user.id, status=StatusEnum.VALID.value)) >= 1:
            return get_data_error_result(
                message="Duplicated dataset name.")

        del update_dict["kb_id"]
        connectors = []
        if "connectors" in update_dict:
            connectors = update_dict["connectors"]
            del update_dict["connectors"]
        if not KnowledgebaseService.update_by_id(kb.id, update_dict):
            return get_data_error_result()

        if kb.pagerank != update_dict.get("pagerank", 0):
            if update_dict.get("pagerank", 0) > 0:
                await thread_pool_exec(
                    settings.docStoreConn.update,
                    {"kb_id": kb.id},
                    {PAGERANK_FLD: update_dict["pagerank"]},
                    search.index_name(kb.tenant_id),
                    kb.id,
                )
            else:
                # Elasticsearch requires PAGERANK_FLD be non-zero!
                await thread_pool_exec(
                    settings.docStoreConn.update,
                    {"exists": PAGERANK_FLD},
                    {"remove": PAGERANK_FLD},
                    search.index_name(kb.tenant_id),
                    kb.id,
                )

        e, kb = KnowledgebaseService.get_by_id(kb.id)
        if not e:
            return get_data_error_result(
                message="Database error (Knowledgebase rename)!")
        errors = Connector2KbService.link_connectors(kb.id, [conn for conn in connectors], current_user.id)
        if errors:
            logging.error("Link KB errors: ", errors)
        kb = kb.to_dict()
        kb.update(update_dict)
        kb["connectors"] = connectors

        return get_json_result(data=kb)
    except Exception as e:
        return server_error_response(e)
"""

"""
Deprecated, todo delete
@manager.route('/list', methods=['POST'])  # noqa: F821
@login_required
async def list_kbs():
    args = request.args
    keywords = args.get("keywords", "")
    page_number = int(args.get("page", 0))
    items_per_page = int(args.get("page_size", 0))
    parser_id = args.get("parser_id")
    orderby = args.get("orderby", "create_time")
    if args.get("desc", "true").lower() == "false":
        desc = False
    else:
        desc = True

    req = await get_request_json()
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
                kbs = kbs[(page_number-1)*items_per_page:page_number*items_per_page]
        return get_json_result(data={"kbs": kbs, "total": total})
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['post'])  # noqa: F821
@login_required
@validate_request("kb_id")
async def rm():
    req = await get_request_json()
    uid = current_user.id
    if not KnowledgebaseService.accessible4deletion(req["kb_id"], uid):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR
        )
    try:
        kbs = KnowledgebaseService.query(
            created_by=uid, id=req["kb_id"])
        if not kbs:
            return get_json_result(
                data=False, message='Only owner of dataset authorized for this operation.',
                code=RetCode.OPERATING_ERROR)

        def _rm_sync():
            for doc in DocumentService.query(kb_id=req["kb_id"]):
                if not DocumentService.remove_document(doc, kbs[0].tenant_id):
                    return get_data_error_result(
                        message="Database error (Document removal)!")
                f2d = File2DocumentService.get_by_document_id(doc.id)
                if f2d:
                    FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
                File2DocumentService.delete_by_document_id(doc.id)
            FileService.filter_delete(
                [
                    File.tenant_id == kbs[0].tenant_id,
                    File.source_type == FileSource.KNOWLEDGEBASE,
                    File.type == "folder",
                    File.name == kbs[0].name,
                ]
            )
            # Delete the table BEFORE deleting the database record
            for kb in kbs:
                try:
                    settings.docStoreConn.delete({"kb_id": kb.id}, search.index_name(kb.tenant_id), kb.id)
                    settings.docStoreConn.delete_idx(search.index_name(kb.tenant_id), kb.id)
                    logging.info(f"Dropped index for dataset {kb.id}")
                except Exception as e:
                    logging.error(f"Failed to drop index for dataset {kb.id}: {e}")

            if not KnowledgebaseService.delete_by_id(req["kb_id"]):
                return get_data_error_result(
                    message="Database error (Knowledgebase removal)!")
            for kb in kbs:
                if hasattr(settings.STORAGE_IMPL, 'remove_bucket'):
                    settings.STORAGE_IMPL.remove_bucket(kb.id)
            return get_json_result(data=True)

        return await thread_pool_exec(_rm_sync)
    except Exception as e:
        return server_error_response(e)
"""

"""
Deprecated, todo delete
@manager.route('/<kb_id>/knowledge_graph', methods=['GET'])  # noqa: F821
@login_required
async def knowledge_graph(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR
        )
    _, kb = KnowledgebaseService.get_by_id(kb_id)
    req = {
        "kb_id": [kb_id],
        "knowledge_graph_kwd": ["graph"]
    }

    obj = {"graph": {}, "mind_map": {}}
    if not settings.docStoreConn.index_exist(search.index_name(kb.tenant_id), kb_id):
        return get_json_result(data=obj)
    sres = await settings.retriever.search(req, search.index_name(kb.tenant_id), [kb_id])
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
            node_id_set = { o["id"] for o in obj["graph"]["nodes"] }
            filtered_edges = [o for o in obj["graph"]["edges"] if o["source"] != o["target"] and o["source"] in node_id_set and o["target"] in node_id_set]
            obj["graph"]["edges"] = sorted(filtered_edges, key=lambda x: x.get("weight", 0), reverse=True)[:128]
    return get_json_result(data=obj)


@manager.route('/<kb_id>/knowledge_graph', methods=['DELETE'])  # noqa: F821
@login_required
def delete_knowledge_graph(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR
        )
    _, kb = KnowledgebaseService.get_by_id(kb_id)
    settings.docStoreConn.delete({"knowledge_graph_kwd": ["graph", "subgraph", "entity", "relation"]}, search.index_name(kb.tenant_id), kb_id)

    return get_json_result(data=True)
"""

"""
Deprecated, todo delete
@manager.route("/run_graphrag", methods=["POST"])  # noqa: F821
@login_required
async def run_graphrag():
    req = await get_request_json()

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
            return get_error_data_result(message=f"Task {task_id} in progress with status {task.progress}. A Graph Task is already running.")

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

    task_id = queue_raptor_o_graphrag_tasks(sample_doc_id=sample_document, ty="graphrag", priority=0, fake_doc_id=GRAPH_RAPTOR_FAKE_DOC_ID, doc_ids=list(document_ids))

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
        return get_json_result(data={})

    return get_json_result(data=task.to_dict())


@manager.route("/run_raptor", methods=["POST"])  # noqa: F821
@login_required
async def run_raptor():
    req = await get_request_json()

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
            return get_error_data_result(message=f"Task {task_id} in progress with status {task.progress}. A RAPTOR Task is already running.")

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

    task_id = queue_raptor_o_graphrag_tasks(sample_doc_id=sample_document, ty="raptor", priority=0, fake_doc_id=GRAPH_RAPTOR_FAKE_DOC_ID, doc_ids=list(document_ids))

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
"""
