from flask import request

from api.db import StatusEnum
from api.db.db_models import APIToken
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import TenantService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import get_json_result
from api.utils.api_utils import server_error_response, get_data_error_result


@manager.route('/save', methods=['POST'])
def save():
    req = request.json
    try:
        token = request.headers.get('Authorization').split()[1]
        objs = APIToken.query(token=token)
        if not objs:
            return get_json_result(
                data=False, retmsg='Token is not valid!"', retcode=RetCode.AUTHENTICATION_ERROR)
        tenant_id = objs[0].tenant_id
        e, t = TenantService.get_by_id(tenant_id)
        if not e:
            return get_data_error_result(retmsg="Tenant not found.")
        if "id" not in req:
            req['id'] = get_uuid()
            req["name"] = req["name"].strip()
            if req["name"] == "":
                return get_data_error_result(
                    retmsg="Name is not empty")
            if KnowledgebaseService.query(name=req["name"]):
                return get_data_error_result(
                    retmsg="Duplicated knowledgebase name")
            req["tenant_id"] = tenant_id
            req['created_by'] = tenant_id
            req['embd_id'] = t.embd_id
            if not KnowledgebaseService.save(**req):
                return get_data_error_result(retmsg="Data saving error")
            del req["created_by"]
            req["embedding_model"] = req['embd_id']
            del req['embd_id']
            return get_json_result(data=req)
        else:
            if req["tenant_id"] != tenant_id or req["embd_id"] != t.embd_id :
                return get_data_error_result(
                    retmsg="Can't change tenant_id or embedding_model")
            e, kb = KnowledgebaseService.get_by_id(req["id"])
            if not e:
                return get_data_error_result(
                    retmsg="Can't find this knowledgebase!")

            if not KnowledgebaseService.query(
                    created_by=tenant_id, id=req["id"]):
                return get_json_result(
                    data=False, retmsg=f'Only owner of knowledgebase authorized for this operation.',
                    retcode=RetCode.OPERATING_ERROR)

            if req["name"].lower() != kb.name.lower() \
                    and len(KnowledgebaseService.query(name=req["name"], tenant_id=req['tenant_id'],
                                                       status=StatusEnum.VALID.value)) > 0:
                return get_data_error_result(
                    retmsg="Duplicated knowledgebase name.")

            del req["id"]
            req['created_by'] = tenant_id
            if not KnowledgebaseService.update_by_id(kb.id, req):
                return get_data_error_result(retmsg="Data update error ")
            return get_json_result(data=True)

    except Exception as e:
        return server_error_response(e)
