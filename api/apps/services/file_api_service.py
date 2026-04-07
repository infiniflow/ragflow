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
import os
import pathlib

from api.common.check_team_permission import check_file_team_permission
from api.db import FileType
from api.db.services import duplicate_name
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.utils.file_utils import filename_type
from common import settings
from common.constants import FileSource
from common.misc_utils import get_uuid, thread_pool_exec


async def upload_file(tenant_id: str, pf_id: str, file_objs: list):
    """
    Upload files to a folder.

    :param tenant_id: tenant ID
    :param pf_id: parent folder ID
    :param file_objs: list of file objects from request
    :return: (success, result_list) or (success, error_message)
    """
    if not pf_id:
        root_folder = FileService.get_root_folder(tenant_id)
        pf_id = root_folder["id"]

    e, pf_folder = FileService.get_by_id(pf_id)
    if not e:
        return False, "Can't find this folder!"

    file_res = []
    for file_obj in file_objs:
        MAX_FILE_NUM_PER_USER = int(os.environ.get('MAX_FILE_NUM_PER_USER', 0))
        if 0 < MAX_FILE_NUM_PER_USER <= await thread_pool_exec(DocumentService.get_doc_count, tenant_id):
            return False, "Exceed the maximum file number of a free user!"

        if not file_obj.filename:
            file_obj_names = [pf_folder.name, file_obj.filename]
        else:
            full_path = '/' + file_obj.filename
            file_obj_names = full_path.split('/')
        file_len = len(file_obj_names)

        file_id_list = await thread_pool_exec(FileService.get_id_list_by_id, pf_id, file_obj_names, 1, [pf_id])
        len_id_list = len(file_id_list)

        if file_len != len_id_list:
            e, file = await thread_pool_exec(FileService.get_by_id, file_id_list[len_id_list - 1])
            if not e:
                return False, "Folder not found!"
            last_folder = await thread_pool_exec(
                FileService.create_folder, file, file_id_list[len_id_list - 1], file_obj_names, len_id_list, tenant_id, tenant_id
            )
        else:
            e, file = await thread_pool_exec(FileService.get_by_id, file_id_list[len_id_list - 2])
            if not e:
                return False, "Folder not found!"
            last_folder = await thread_pool_exec(
                FileService.create_folder, file, file_id_list[len_id_list - 2], file_obj_names, len_id_list, tenant_id, tenant_id
            )

        filetype = filename_type(file_obj_names[file_len - 1])
        location = file_obj_names[file_len - 1]
        while await thread_pool_exec(settings.STORAGE_IMPL.obj_exist, last_folder.id, location):
            location += "_"
        blob = await thread_pool_exec(file_obj.read)
        filename = await thread_pool_exec(
            duplicate_name, FileService.query, name=file_obj_names[file_len - 1], parent_id=last_folder.id
        )
        await thread_pool_exec(settings.STORAGE_IMPL.put, last_folder.id, location, blob)
        file_data = {
            "id": get_uuid(),
            "parent_id": last_folder.id,
            "tenant_id": tenant_id,
            "created_by": tenant_id,
            "type": filetype,
            "name": filename,
            "location": location,
            "size": len(blob),
        }
        inserted = await thread_pool_exec(FileService.insert, file_data)
        file_res.append(inserted.to_json())

    return True, file_res


async def create_folder(tenant_id: str, name: str, pf_id: str = None, file_type: str = None):
    """
    Create a new folder or virtual file.

    :param tenant_id: tenant ID
    :param name: folder name
    :param pf_id: parent folder ID
    :param file_type: file type (folder or virtual)
    :return: (success, result) or (success, error_message)
    """
    if not pf_id:
        root_folder = FileService.get_root_folder(tenant_id)
        pf_id = root_folder["id"]

    if not FileService.is_parent_folder_exist(pf_id):
        return False, "Parent Folder Doesn't Exist!"
    if FileService.query(name=name, parent_id=pf_id):
        return False, "Duplicated folder name in the same folder."

    if file_type == FileType.FOLDER.value:
        ft = FileType.FOLDER.value
    else:
        ft = FileType.VIRTUAL.value

    file = FileService.insert({
        "id": get_uuid(),
        "parent_id": pf_id,
        "tenant_id": tenant_id,
        "created_by": tenant_id,
        "name": name,
        "location": "",
        "size": 0,
        "type": ft,
    })
    return True, file.to_json()


def list_files(tenant_id: str, args: dict):
    """
    List files under a folder.

    :param tenant_id: tenant ID
    :param args: query arguments (parent_id, keywords, page, page_size, orderby, desc)
    :return: (success, result) or (success, error_message)
    """
    pf_id = args.get("parent_id")
    keywords = args.get("keywords", "")
    page_number = int(args.get("page", 1))
    items_per_page = int(args.get("page_size", 15))
    orderby = args.get("orderby", "create_time")
    desc = args.get("desc", True)

    if not pf_id:
        root_folder = FileService.get_root_folder(tenant_id)
        pf_id = root_folder["id"]
        FileService.init_knowledgebase_docs(pf_id, tenant_id)
        FileService.init_skills_folder(pf_id, tenant_id)

    e, file = FileService.get_by_id(pf_id)
    if not e:
        return False, "Folder not found!"

    files, total = FileService.get_by_pf_id(tenant_id, pf_id, page_number, items_per_page, orderby, desc, keywords)

    parent_folder = FileService.get_parent_folder(pf_id)
    if not parent_folder:
        return False, "File not found!"

    return True, {"total": total, "files": files, "parent_folder": parent_folder.to_json()}



def get_parent_folder(file_id: str):
    """
    Get parent folder of a file.

    :param file_id: file ID
    :return: (success, result) or (success, error_message)
    """
    e, file = FileService.get_by_id(file_id)
    if not e:
        return False, "Folder not found!"

    parent_folder = FileService.get_parent_folder(file_id)
    return True, {"parent_folder": parent_folder.to_json()}


def get_all_parent_folders(file_id: str):
    """
    Get all ancestor folders of a file.

    :param file_id: file ID
    :return: (success, result) or (success, error_message)
    """
    e, file = FileService.get_by_id(file_id)
    if not e:
        return False, "Folder not found!"

    parent_folders = FileService.get_all_parent_folders(file_id)
    return True, {"parent_folders": [pf.to_json() for pf in parent_folders]}


async def delete_files(uid: str, file_ids: list, auth_header: str = ""):
    """
    Delete files/folders with team permission check and recursive deletion.

    :param uid: user ID
    :param file_ids: list of file IDs to delete
    :param auth_header: Authorization header for Go backend API calls
    :return: (success, result) or (success, error_message)
    """
    def _get_hub_uuid_by_name(tenant_id, hub_name, authorization):
        """Get hub UUID by hub name from Go backend"""
        try:
            import requests

            host = getattr(settings, 'HOST_IP', '127.0.0.1')
            # Go service runs on port+4 (9384 by default)
            port = getattr(settings, 'HOST_PORT', 9380) + 4
            service_url = f"http://{host}:{port}"

            # List all hubs and find the one matching the name
            url = f"{service_url}/api/v1/ns/hubs"
            headers = {"Content-Type": "application/json"}
            if authorization:
                headers["Authorization"] = authorization

            response = requests.get(url, headers=headers, timeout=10)

            if response.status_code == 200:
                data = response.json()
                if data.get("code") == 0:
                    hubs = data.get("data", {}).get("hubs", [])
                    for hub in hubs:
                        if hub.get("name") == hub_name:
                            return hub.get("id")
        except Exception as e:
            logging.warning(f"Error getting hub UUID: {e}")
        return None

    def _delete_skill_index(tenant_id, hub_name, skill_name, authorization):
        """Delete skill index from Go backend.

        Returns:
            bool: True if deletion succeeded (HTTP 200), False otherwise.
        """
        try:
            import requests
            from urllib.parse import quote

            # Construct service URL from settings
            host = getattr(settings, 'HOST_IP', '127.0.0.1')
            # Go service runs on port+4 (9384 by default)
            port = getattr(settings, 'HOST_PORT', 9380) + 4
            service_url = f"http://{host}:{port}"

            # Get hub UUID from hub name
            hub_uuid = _get_hub_uuid_by_name(tenant_id, hub_name, authorization)
            hub_id = hub_uuid if hub_uuid else hub_name

            url = f"{service_url}/api/v1/ns/index?skill_id={quote(skill_name)}&hub_id={quote(hub_id)}"
            headers = {"Content-Type": "application/json"}
            if authorization:
                headers["Authorization"] = authorization

            response = requests.delete(url, headers=headers, timeout=10)
            if response.status_code == 200:
                try:
                    data = response.json()
                    if data.get("code") == 0:
                        logging.info(
                            f"Successfully deleted skill index: hub={hub_name}, skill={skill_name}, "
                            f"status={response.status_code}, code=0"
                        )
                        return True
                    else:
                        app_code = data.get("code", "unknown")
                        app_msg = data.get("message", "no message")
                        logging.error(
                            f"Failed to delete skill index: hub={hub_name}, skill={skill_name}, "
                            f"status={response.status_code}, app_code={app_code}, app_msg={app_msg}, "
                            f"response={response.text}"
                        )
                        return False
                except ValueError as json_err:
                    # JSON decode error - treat as failure
                    logging.error(
                        f"Failed to parse delete response JSON: hub={hub_name}, skill={skill_name}, "
                        f"error={json_err}, raw_response={response.text}"
                    )
                    return False
            else:
                logging.error(
                    f"Failed to delete skill index: hub={hub_name}, skill={skill_name}, "
                    f"status={response.status_code}, response={response.text}"
                )
                return False
        except Exception as e:
            logging.error(
                f"Exception deleting skill index: hub={hub_name}, skill={skill_name}, error={e}"
            )
            return False

    def _delete_single_file(file):
        try:
            if file.location:
                settings.STORAGE_IMPL.rm(file.parent_id, file.location)
        except Exception as e:
            logging.exception(f"Fail to remove object: {file.parent_id}/{file.location}, error: {e}")

        informs = File2DocumentService.get_by_file_id(file.id)
        for inform in informs:
            doc_id = inform.document_id
            e, doc = DocumentService.get_by_id(doc_id)
            if e and doc:
                tenant_id = DocumentService.get_tenant_id(doc_id)
                if tenant_id:
                    DocumentService.remove_document(doc, tenant_id)
            File2DocumentService.delete_by_file_id(file.id)

        FileService.delete(file)

    def _find_ancestor_skills_hub(folder_id, tenant_id):
        """Walk up the folder hierarchy to find an ancestor with source_type == 'skills_hub'.

        Returns:
            tuple: (success, folder) where folder has source_type == 'skills_hub', or (False, None)
        """
        visited = set()
        current_id = folder_id
        while current_id and current_id not in visited:
            visited.add(current_id)
            success, folder = FileService.get_by_id(current_id)
            if not success or not folder:
                return False, None
            if folder.source_type == "skills_hub":
                return True, folder
            # Move to parent
            current_id = folder.parent_id
        return False, None

    def _delete_folder_recursive(folder, tenant_id, is_skill_folder=False, hub_name=None, authorization=""):
        # Check if this is a skill folder or has a skills_hub ancestor
        current_hub_name = hub_name
        if not is_skill_folder and not current_hub_name:
            # First check immediate parent
            parent_success, parent_folder = FileService.get_by_id(folder.parent_id)
            if parent_success and parent_folder and parent_folder.source_type == "skills_hub":
                is_skill_folder = True
                # Use parent folder name as hub name (e.g., "hub11")
                current_hub_name = parent_folder.name
            else:
                # Walk up the hierarchy to find skills_hub ancestor
                ancestor_success, ancestor_folder = _find_ancestor_skills_hub(folder.parent_id, tenant_id)
                if ancestor_success and ancestor_folder:
                    is_skill_folder = True
                    current_hub_name = ancestor_folder.name

        # If this is a skill folder, delete its index first
        if is_skill_folder and current_hub_name:
            index_deleted = _delete_skill_index(tenant_id, current_hub_name, folder.name, authorization)
            if not index_deleted:
                logging.error(
                    f"Aborting folder deletion due to index deletion failure: "
                    f"folder={folder.name}, hub={current_hub_name}"
                )
                raise RuntimeError(
                    f"Failed to delete skill index for folder '{folder.name}' in hub '{current_hub_name}'. "
                    f"Folder deletion aborted to prevent orphaned indexes."
                )

        sub_files = FileService.list_all_files_by_parent_id(folder.id)
        for sub_file in sub_files:
            if sub_file.type == FileType.FOLDER.value:
                _delete_folder_recursive(sub_file, tenant_id, is_skill_folder, current_hub_name, authorization)
            else:
                # If we're in a skill folder context, delete the skill index for this file too
                if is_skill_folder and current_hub_name:
                    index_deleted = _delete_skill_index(tenant_id, current_hub_name, sub_file.name, authorization)
                    if not index_deleted:
                        logging.error(
                            f"Aborting file deletion due to index deletion failure: "
                            f"file={sub_file.name}, hub={current_hub_name}"
                        )
                        raise RuntimeError(
                            f"Failed to delete skill index for file '{sub_file.name}' in hub '{current_hub_name}'. "
                            f"File deletion aborted to prevent orphaned indexes."
                        )
                _delete_single_file(sub_file)
        FileService.delete(folder)

    def _rm_sync():
        for file_id in file_ids:
            e, file = FileService.get_by_id(file_id)
            if not e or not file:
                return False, "File or Folder not found!"
            if not file.tenant_id:
                return False, "Tenant not found!"
            if not check_file_team_permission(file, uid):
                return False, "No authorization."

            if file.source_type == FileSource.KNOWLEDGEBASE:
                continue

            if file.type == FileType.FOLDER.value:
                try:
                    _delete_folder_recursive(file, uid, False, None, auth_header)
                except RuntimeError as e:
                    logging.error(f"Folder deletion failed: {e}")
                    return False, str(e)
                continue

            _delete_single_file(file)

        return True, True

    return await thread_pool_exec(_rm_sync)


async def move_files(uid: str, src_file_ids: list, dest_file_id: str = None, new_name: str = None):
    """
    Move and/or rename files. Follows Linux mv semantics:
    - new_name only: rename in place (no storage operation)
    - dest_file_id only: move to new folder (keep names)
    - both: move and rename simultaneously

    :param uid: user ID
    :param src_file_ids: list of source file IDs
    :param dest_file_id: destination folder ID (optional)
    :param new_name: new name for the file (optional, single file only)
    :return: (success, result) or (success, error_message)
    """
    files = FileService.get_by_ids(src_file_ids)
    if not files:
        return False, "Source files not found!"

    files_dict = {f.id: f for f in files}

    for file_id in src_file_ids:
        file = files_dict.get(file_id)
        if not file:
            return False, "File or folder not found!"
        if not file.tenant_id:
            return False, "Tenant not found!"
        if not check_file_team_permission(file, uid):
            return False, "No authorization."

    dest_folder = None
    if dest_file_id:
        ok, dest_folder = FileService.get_by_id(dest_file_id)
        if not ok or not dest_folder:
            return False, "Parent folder not found!"

    if new_name:
        file = files_dict[src_file_ids[0]]
        if file.type != FileType.FOLDER.value and \
                pathlib.Path(new_name.lower()).suffix != pathlib.Path(file.name.lower()).suffix:
            return False, "The extension of file can't be changed"
        target_parent_id = dest_folder.id if dest_folder else file.parent_id
        for f in FileService.query(name=new_name, parent_id=target_parent_id):
            if f.name == new_name:
                return False, "Duplicated file name in the same folder."

    def _move_entry_recursive(source_file_entry, dest_folder_entry, override_name=None):
        effective_name = override_name or source_file_entry.name

        if source_file_entry.type == FileType.FOLDER.value:
            existing_folder = FileService.query(name=effective_name, parent_id=dest_folder_entry.id)
            if existing_folder:
                new_folder = existing_folder[0]
            else:
                new_folder = FileService.insert({
                    "id": get_uuid(),
                    "parent_id": dest_folder_entry.id,
                    "tenant_id": source_file_entry.tenant_id,
                    "created_by": source_file_entry.tenant_id,
                    "name": effective_name,
                    "location": "",
                    "size": 0,
                    "type": FileType.FOLDER.value,
                })

            sub_files = FileService.list_all_files_by_parent_id(source_file_entry.id)
            for sub_file in sub_files:
                _move_entry_recursive(sub_file, new_folder)

            FileService.delete_by_id(source_file_entry.id)
            return

        # Non-folder file
        need_storage_move = dest_folder_entry.id != source_file_entry.parent_id
        updates = {}

        if need_storage_move:
            new_location = effective_name
            while settings.STORAGE_IMPL.obj_exist(dest_folder_entry.id, new_location):
                new_location += "_"
            try:
                settings.STORAGE_IMPL.move(
                    source_file_entry.parent_id, source_file_entry.location,
                    dest_folder_entry.id, new_location,
                )
            except Exception as storage_err:
                raise RuntimeError(f"Move file failed at storage layer: {str(storage_err)}")
            updates["parent_id"] = dest_folder_entry.id
            updates["location"] = new_location

        if override_name:
            updates["name"] = override_name

        if updates:
            FileService.update_by_id(source_file_entry.id, updates)

        if override_name:
            informs = File2DocumentService.get_by_file_id(source_file_entry.id)
            if informs:
                if not DocumentService.update_by_id(informs[0].document_id, {"name": override_name}):
                    raise RuntimeError("Database error (Document rename)!")

    def _move_or_rename_sync():
        if dest_folder:
            for file in files:
                _move_entry_recursive(file, dest_folder, override_name=new_name)
        else:
            # Pure rename: no storage operation needed
            file = files[0]
            if not FileService.update_by_id(file.id, {"name": new_name}):
                return False, "Database error (File rename)!"
            informs = File2DocumentService.get_by_file_id(file.id)
            if informs:
                if not DocumentService.update_by_id(informs[0].document_id, {"name": new_name}):
                    return False, "Database error (Document rename)!"
        return True, True

    return await thread_pool_exec(_move_or_rename_sync)


def get_file_content(uid: str, file_id: str):
    """
    Get file content and metadata for download.

    :param uid: user ID
    :param file_id: file ID
    :return: (success, (blob, file_obj)) or (success, error_message)
    """
    e, file = FileService.get_by_id(file_id)
    if not e:
        return False, "Document not found!"
    if not check_file_team_permission(file, uid):
        return False, "No authorization."
    return True, file
