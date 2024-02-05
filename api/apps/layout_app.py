import re

import numpy as np
from PIL import Image
from flask import request, session, redirect, url_for
from werkzeug.security import generate_password_hash, check_password_hash
from flask_login import login_required, current_user, login_user, logout_user

from api.db.db_models import TenantLLM
from api.db.services.llm_service import TenantLLMService, LLMService
from api.utils.api_utils import server_error_response, validate_request, get_data_error_result
from api.utils import get_uuid, get_format_time, decrypt, download_img
from api.db import UserTenantRole, LLMType, ParserType
from api.settings import RetCode, GITHUB_OAUTH, CHAT_MDL, EMBEDDING_MDL, ASR_MDL, IMAGE2TEXT_MDL, PARSERS
from api.db.services.user_service import UserService, TenantService, UserTenantService
from api.settings import stat_logger
from api.utils.api_utils import get_json_result, cors_reponse
from rag.cv.ppdetection import PPDet

ppdet = PPDet("/data/newpeak/medical-gpt/res/ppdet")
tbl_ppdet = PPDet("/data/newpeak/medical-gpt/res/ppdet.tbl")

LAYOUT_FACTORY = {
    ParserType.GENERAL.value: ppdet,
    ParserType.PAPER.value: PPDet("/data/newpeak/medical-gpt/res/ppdet.paper"),
    ParserType.PRESENTATION.value: ppdet,
    ParserType.MANUAL.value: ppdet,
    ParserType.LAWS.value: ppdet,
    ParserType.QA.value: ppdet,
    "table_component": ppdet,
}

@manager.route("/detect/<model_name>", methods=['POST'])
def detect(model_name):
    token = request.headers.get("Authorization")
    if not token:
        return get_json_result(data=False, retcode=RetCode.AUTHENTICATION_ERROR,
                               retmsg='Unautherized!')
    thr = float(request.form.get("threashold", 0.7))
    files = request.files.to_dict(flat=False)["image"]
    try:
        e, t = TenantService.get_by_id(token)
        if not e:
            return get_json_result(data=False, retcode=RetCode.AUTHENTICATION_ERROR,
                               retmsg='Unautherized!')
        if t.credit <= len(files):
            return get_data_error_result(retmsg="Credit run out! Concat media@infiniflow.org")
    except Exception as ee:
        return server_error_response(ee)

    if model_name not in LAYOUT_FACTORY:
        return get_data_error_result(retmsg="{model_name} not found!")

    res = LAYOUT_FACTORY[model_name]([np.array(Image.open(file.stream)) for file in files], thr)
    TenantService.decrease(token, len(files))
    return get_json_result(data=res)
