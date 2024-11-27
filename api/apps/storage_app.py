import os

from flask import request
from flask_login import login_required, current_user

from api.db import FileType
from api.db.services.file_service import FileService
from api.utils import get_uuid
from api.utils.api_utils import get_json_result
from api.utils.api_utils import server_error_response, get_data_error_result
from api.utils.file_utils import filename_type
from rag.utils.storage_factory import STORAGE_IMPL


@manager.route('/list', methods=['GET'])
def list_storage_keys():
    dir = request.args.get("dir", "/")
    try:
        def filter_dir_and_exist_file(key):
            if key["size"] == 0:
                return False
            parent_dir = os.path.dirname(key["name"])
            file_name = os.path.basename(key["name"])
            dirs = list(filter(None, parent_dir.split("/")))
            user_root_folder = FileService.get_root_folder(current_user.id)
            pf_id = user_root_folder["id"]
            for dir in dirs:
                exist_file = FileService.get_by_pf_id_name(id=pf_id, name=dir)
                if not exist_file:
                    return True
                pf_id = exist_file.id
            exist_file = FileService.get_by_pf_id_name(id=pf_id, name=file_name)
            return exist_file == None

        files = STORAGE_IMPL.list(None, dir)
        return get_json_result(data=list(filter(filter_dir_and_exist_file, files)))
    except Exception as e:
        return server_error_response(e)


@manager.route('/import', methods=['POST'])
@login_required
def import_storage_keys():
    content = request.json
    keys = content['keys']
    is_dir = content['dir']
    try:
        all_keys = []
        file_res = []
        if is_dir:
            for key in keys:
                all_keys = all_keys + STORAGE_IMPL.list(bucket=None, dir=key, recursive=True)
        else:
            for key in keys:
                all_keys.append(key)

        for key in all_keys:
            parent_dir = os.path.dirname(key)
            file_name = os.path.basename(key)

            dirs = list(filter(None, parent_dir.split("/")))
            user_root_folder = FileService.get_root_folder(current_user.id)
            pf_id = user_root_folder["id"]
            for dir in dirs:
                exist_file = FileService.get_by_pf_id_name(id=pf_id, name=dir)
                if exist_file:
                    pf_id = exist_file.id
                    continue
                file = FileService.insert({
                    "id": get_uuid(),
                    "parent_id": pf_id,
                    "tenant_id": current_user.id,
                    "created_by": current_user.id,
                    "name": dir,
                    "location": "",
                    "size": 0,
                    "type": FileType.FOLDER.value
                })
                pf_id = file.id

            e, file = FileService.get_by_id(pf_id)
            if not e:
                return get_data_error_result(
                    retmsg="Can't find this folder!")

            if FileService.get_by_pf_id_name(id=pf_id, name=file_name):
                continue

            filetype = filename_type(file_name)
            location = key
            file = {
                "id": get_uuid(),
                "parent_id": pf_id,
                "tenant_id": current_user.id,
                "created_by": current_user.id,
                "type": filetype,
                "name": file_name,
                "location": location,
                "size": STORAGE_IMPL.get_properties("bucket", key)["size"],
            }
            file = FileService.insert(file)
            file_res.append(file.to_json())

        return get_json_result(data=content)
    except Exception as e:
        return server_error_response(e)
