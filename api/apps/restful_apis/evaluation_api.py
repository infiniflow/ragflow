#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
import logging

from quart import request

from api.apps import current_user, login_required
from api.db.services.dialog_service import DialogService
from api.db.services.evaluation_service import EvaluationService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils.api_utils import get_error_argument_result, get_json_result, get_request_json, validate_request
from common.constants import RetCode, StatusEnum


def _tenant_id() -> str:
    return current_user.id


def _dataset_or_404(dataset_id: str):
    dataset = EvaluationService._get_dataset_for_tenant(dataset_id, _tenant_id())
    if not dataset:
        return None, get_json_result(code=RetCode.NOT_FOUND, message="Evaluation dataset not found")
    return dataset, None


def _run_or_404(run_id: str):
    run = EvaluationService.get_run(run_id)
    if not run:
        return None, get_json_result(code=RetCode.NOT_FOUND, message="Evaluation run not found")
    dataset = EvaluationService._get_dataset_for_tenant(run["dataset_id"], _tenant_id())
    if not dataset:
        return None, get_json_result(code=RetCode.NOT_FOUND, message="Evaluation run not found")
    return run, None


def _validate_kb_ids(kb_ids: list) -> bool:
    if not kb_ids:
        return False
    tenant_id = _tenant_id()
    for kb_id in kb_ids:
        ok, kb = KnowledgebaseService.get_by_id(kb_id)
        if not ok or kb.tenant_id != tenant_id or kb.status != StatusEnum.VALID.value:
            return False
    return True


@manager.route("/evaluations/datasets", methods=["GET"])  # noqa: F821
@login_required
async def list_evaluation_datasets():
    page = int(request.args.get("page", 1))
    page_size = int(request.args.get("page_size", 20))
    try:
        data = EvaluationService.list_datasets(_tenant_id(), current_user.id, page, page_size)
        return get_json_result(data=data)
    except Exception as e:
        logging.exception(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message=str(e))


@manager.route("/evaluations/datasets", methods=["POST"])  # noqa: F821
@login_required
@validate_request("name", "kb_ids")
async def create_evaluation_dataset():
    req = await get_request_json()
    kb_ids = req.get("kb_ids") or []
    if not _validate_kb_ids(kb_ids):
        return get_error_argument_result("Invalid or inaccessible knowledge base ids")
    try:
        ok, res = EvaluationService.create_dataset(
            name=req["name"],
            description=req.get("description", ""),
            kb_ids=kb_ids,
            tenant_id=_tenant_id(),
            user_id=current_user.id,
        )
        if not ok:
            return get_json_result(code=RetCode.SERVER_ERROR, message=res)
        return get_json_result(data={"id": res})
    except Exception as e:
        logging.exception(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message=str(e))


@manager.route("/evaluations/datasets/<dataset_id>", methods=["GET"])  # noqa: F821
@login_required
async def get_evaluation_dataset(dataset_id):
    dataset, err = _dataset_or_404(dataset_id)
    if err:
        return err
    return get_json_result(data=dataset)


@manager.route("/evaluations/datasets/<dataset_id>", methods=["PUT"])  # noqa: F821
@login_required
async def update_evaluation_dataset(dataset_id):
    dataset, err = _dataset_or_404(dataset_id)
    if err:
        return err
    req = await get_request_json()
    updates = {}
    if "name" in req:
        updates["name"] = req["name"]
    if "description" in req:
        updates["description"] = req["description"]
    if "kb_ids" in req:
        if not _validate_kb_ids(req["kb_ids"]):
            return get_error_argument_result("Invalid or inaccessible knowledge base ids")
        updates["kb_ids"] = req["kb_ids"]
    if not updates:
        return get_error_argument_result("No fields to update")
    try:
        if not EvaluationService.update_dataset(dataset_id, **updates):
            return get_json_result(code=RetCode.SERVER_ERROR, message="Failed to update dataset")
        return get_json_result(data=EvaluationService.get_dataset(dataset_id))
    except Exception as e:
        logging.exception(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message=str(e))


@manager.route("/evaluations/datasets/<dataset_id>", methods=["DELETE"])  # noqa: F821
@login_required
async def delete_evaluation_dataset(dataset_id):
    dataset, err = _dataset_or_404(dataset_id)
    if err:
        return err
    try:
        if not EvaluationService.delete_dataset(dataset_id):
            return get_json_result(code=RetCode.SERVER_ERROR, message="Failed to delete dataset")
        return get_json_result()
    except Exception as e:
        logging.exception(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message=str(e))


@manager.route("/evaluations/datasets/<dataset_id>/cases", methods=["GET"])  # noqa: F821
@login_required
async def list_evaluation_cases(dataset_id):
    dataset, err = _dataset_or_404(dataset_id)
    if err:
        return err
    try:
        cases = EvaluationService.get_test_cases(dataset_id)
        return get_json_result(data={"cases": cases, "total": len(cases)})
    except Exception as e:
        logging.exception(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message=str(e))


@manager.route("/evaluations/datasets/<dataset_id>/cases", methods=["POST"])  # noqa: F821
@login_required
@validate_request("question")
async def add_evaluation_case(dataset_id):
    dataset, err = _dataset_or_404(dataset_id)
    if err:
        return err
    req = await get_request_json()
    case_metadata = req.get("case_metadata") or req.get("metadata")
    try:
        ok, case_id = EvaluationService.add_test_case(
            dataset_id=dataset_id,
            question=req["question"],
            reference_answer=req.get("reference_answer"),
            relevant_doc_ids=req.get("relevant_doc_ids"),
            relevant_chunk_ids=req.get("relevant_chunk_ids"),
            metadata=case_metadata,
        )
        if not ok:
            return get_json_result(code=RetCode.SERVER_ERROR, message=case_id)
        return get_json_result(data={"id": case_id})
    except Exception as e:
        logging.exception(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message=str(e))


@manager.route("/evaluations/datasets/<dataset_id>/cases/import", methods=["POST"])  # noqa: F821
@login_required
@validate_request("cases")
async def import_evaluation_cases(dataset_id):
    dataset, err = _dataset_or_404(dataset_id)
    if err:
        return err
    req = await get_request_json()
    cases = req.get("cases") or []
    normalized = []
    for item in cases:
        if not item.get("question"):
            continue
        row = dict(item)
        if "case_metadata" in row and "metadata" not in row:
            row["metadata"] = row.pop("case_metadata")
        normalized.append(row)
    try:
        success_count, failure_count = EvaluationService.import_test_cases(dataset_id, normalized)
        return get_json_result(data={
            "success_count": success_count,
            "failure_count": failure_count,
        })
    except Exception as e:
        logging.exception(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message=str(e))


@manager.route("/evaluations/datasets/<dataset_id>/cases/<case_id>", methods=["DELETE"])  # noqa: F821
@login_required
async def delete_evaluation_case(dataset_id, case_id):
    dataset, err = _dataset_or_404(dataset_id)
    if err:
        return err
    try:
        if not EvaluationService.delete_test_case(case_id):
            return get_json_result(code=RetCode.SERVER_ERROR, message="Failed to delete test case")
        return get_json_result()
    except Exception as e:
        logging.exception(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message=str(e))


@manager.route("/evaluations/datasets/<dataset_id>/runs", methods=["GET"])  # noqa: F821
@login_required
async def list_evaluation_runs(dataset_id):
    dataset, err = _dataset_or_404(dataset_id)
    if err:
        return err
    try:
        runs = EvaluationService.list_runs(dataset_id)
        return get_json_result(data={"runs": runs, "total": len(runs)})
    except Exception as e:
        logging.exception(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message=str(e))


@manager.route("/evaluations/datasets/<dataset_id>/runs", methods=["POST"])  # noqa: F821
@login_required
@validate_request("dialog_id")
async def start_evaluation_run(dataset_id):
    dataset, err = _dataset_or_404(dataset_id)
    if err:
        return err
    req = await get_request_json()
    dialog_id = req["dialog_id"]
    ok, dialog = DialogService.get_by_id(dialog_id)
    if not ok or dialog.tenant_id != _tenant_id():
        return get_error_argument_result("Chat assistant not found or inaccessible")
    try:
        success, run_id = EvaluationService.start_evaluation(
            dataset_id=dataset_id,
            dialog_id=dialog_id,
            user_id=current_user.id,
            name=req.get("name"),
            background=True,
        )
        if not success:
            return get_json_result(code=RetCode.SERVER_ERROR, message=run_id)
        return get_json_result(data={"id": run_id})
    except Exception as e:
        logging.exception(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message=str(e))


@manager.route("/evaluations/runs/<run_id>", methods=["GET"])  # noqa: F821
@login_required
async def get_evaluation_run(run_id):
    run, err = _run_or_404(run_id)
    if err:
        return err
    return get_json_result(data=run)


@manager.route("/evaluations/runs/<run_id>/results", methods=["GET"])  # noqa: F821
@login_required
async def get_evaluation_run_results(run_id):
    run, err = _run_or_404(run_id)
    if err:
        return err
    try:
        data = EvaluationService.get_run_results(run_id)
        if not data:
            return get_json_result(code=RetCode.NOT_FOUND, message="Results not found")
        return get_json_result(data=data)
    except Exception as e:
        logging.exception(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message=str(e))


@manager.route("/evaluations/runs/<run_id>/recommendations", methods=["GET"])  # noqa: F821
@login_required
async def get_evaluation_recommendations(run_id):
    run, err = _run_or_404(run_id)
    if err:
        return err
    try:
        recommendations = EvaluationService.get_recommendations(run_id)
        return get_json_result(data={"recommendations": recommendations})
    except Exception as e:
        logging.exception(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message=str(e))
