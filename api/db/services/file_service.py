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
from flask_login import current_user
from peewee import fn

from api.db import FileType, KNOWLEDGEBASE_FOLDER_NAME, FileSource
from api.db.db_models import DB, File2Document, Knowledgebase
from api.db.db_models import File, Document
from api.db.services.common_service import CommonService
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.utils import get_uuid


class FileService(CommonService):
    model = File

    @classmethod
    @DB.connection_context()
    def get_by_pf_id(cls, tenant_id, pf_id, page_number, items_per_page,
                     orderby, desc, keywords):
        if keywords:
            files = cls.model.select().where(
                (cls.model.tenant_id == tenant_id),
                (cls.model.parent_id == pf_id),
                (fn.LOWER(cls.model.name).contains(keywords.lower())),
                ~(cls.model.id == pf_id)
            )
        else:
            files = cls.model.select().where((cls.model.tenant_id == tenant_id),
                                             (cls.model.parent_id == pf_id),
                                             ~(cls.model.id == pf_id)
                                             )
        count = files.count()
        if desc:
            files = files.order_by(cls.model.getter_by(orderby).desc())
        else:
            files = files.order_by(cls.model.getter_by(orderby).asc())

        files = files.paginate(page_number, items_per_page)

        res_files = list(files.dicts())
        for file in res_files:
            if file["type"] == FileType.FOLDER.value:
                file["size"] = cls.get_folder_size(file["id"])
                file['kbs_info'] = []
                continue
            kbs_info = cls.get_kb_id_by_file_id(file['id'])
            file['kbs_info'] = kbs_info

        return res_files, count

    @classmethod
    @DB.connection_context()
    def get_kb_id_by_file_id(cls, file_id):
        kbs = (cls.model.select(*[Knowledgebase.id, Knowledgebase.name])
               .join(File2Document, on=(File2Document.file_id == file_id))
               .join(Document, on=(File2Document.document_id == Document.id))
               .join(Knowledgebase, on=(Knowledgebase.id == Document.kb_id))
               .where(cls.model.id == file_id))
        if not kbs: return []
        kbs_info_list = []
        for kb in list(kbs.dicts()):
            kbs_info_list.append({"kb_id": kb['id'], "kb_name": kb['name']})
        return kbs_info_list

    @classmethod
    @DB.connection_context()
    def get_by_pf_id_name(cls, id, name):
        file = cls.model.select().where((cls.model.parent_id == id) & (cls.model.name == name))
        if file.count():
            e, file = cls.get_by_id(file[0].id)
            if not e:
                raise RuntimeError("Database error (File retrieval)!")
            return file
        return None

    @classmethod
    @DB.connection_context()
    def get_id_list_by_id(cls, id, name, count, res):
        if count < len(name):
            file = cls.get_by_pf_id_name(id, name[count])
            if file:
                res.append(file.id)
                return cls.get_id_list_by_id(file.id, name, count + 1, res)
            else:
                return res
        else:
            return res

    @classmethod
    @DB.connection_context()
    def get_all_innermost_file_ids(cls, folder_id, result_ids):
        subfolders = cls.model.select().where(cls.model.parent_id == folder_id)
        if subfolders.exists():
            for subfolder in subfolders:
                cls.get_all_innermost_file_ids(subfolder.id, result_ids)
        else:
            result_ids.append(folder_id)
        return result_ids

    @classmethod
    @DB.connection_context()
    def create_folder(cls, file, parent_id, name, count):
        if count > len(name) - 2:
            return file
        else:
            file = cls.insert({
                "id": get_uuid(),
                "parent_id": parent_id,
                "tenant_id": current_user.id,
                "created_by": current_user.id,
                "name": name[count],
                "location": "",
                "size": 0,
                "type": FileType.FOLDER.value
            })
            return cls.create_folder(file, file.id, name, count + 1)

    @classmethod
    @DB.connection_context()
    def is_parent_folder_exist(cls, parent_id):
        parent_files = cls.model.select().where(cls.model.id == parent_id)
        if parent_files.count():
            return True
        cls.delete_folder_by_pf_id(parent_id)
        return False

    @classmethod
    @DB.connection_context()
    def get_root_folder(cls, tenant_id):
        for file in cls.model.select().where((cls.model.tenant_id == tenant_id),
                                        (cls.model.parent_id == cls.model.id)
                                        ):
            return file.to_dict()

        file_id = get_uuid()
        file = {
            "id": file_id,
            "parent_id": file_id,
            "tenant_id": tenant_id,
            "created_by": tenant_id,
            "name": "/",
            "type": FileType.FOLDER.value,
            "size": 0,
            "location": "",
        }
        cls.save(**file)
        return file

    @classmethod
    @DB.connection_context()
    def get_kb_folder(cls, tenant_id):
        for root in cls.model.select().where(cls.model.tenant_id == tenant_id and
                                             cls.model.parent_id == cls.model.id):
            for folder in cls.model.select().where(cls.model.tenant_id == tenant_id and
                                     cls.model.parent_id == root.id and
                                     cls.model.name == KNOWLEDGEBASE_FOLDER_NAME
                                     ):
                return folder.to_dict()
        assert False, "Can't find the KB folder. Database init error."

    @classmethod
    @DB.connection_context()
    def new_a_file_from_kb(cls, tenant_id, name, parent_id, ty=FileType.FOLDER.value, size=0, location=""):
        for file in cls.query(tenant_id=tenant_id, parent_id=parent_id, name=name):
            return file.to_dict()
        file = {
            "id": get_uuid(),
            "parent_id": parent_id,
            "tenant_id": tenant_id,
            "created_by": tenant_id,
            "name": name,
            "type": ty,
            "size": size,
            "location": location,
            "source_type": FileSource.KNOWLEDGEBASE
        }
        cls.save(**file)
        return file

    @classmethod
    @DB.connection_context()
    def init_knowledgebase_docs(cls, root_id, tenant_id):
        for _ in cls.model.select().where((cls.model.name == KNOWLEDGEBASE_FOLDER_NAME)\
                                          & (cls.model.parent_id == root_id)):
            return
        folder = cls.new_a_file_from_kb(tenant_id, KNOWLEDGEBASE_FOLDER_NAME, root_id)

        for kb in Knowledgebase.select(*[Knowledgebase.id, Knowledgebase.name]).where(Knowledgebase.tenant_id==tenant_id):
            kb_folder = cls.new_a_file_from_kb(tenant_id, kb.name, folder["id"])
            for doc in DocumentService.query(kb_id=kb.id):
                FileService.add_file_from_kb(doc.to_dict(), kb_folder["id"], tenant_id)

    @classmethod
    @DB.connection_context()
    def get_parent_folder(cls, file_id):
        file = cls.model.select().where(cls.model.id == file_id)
        if file.count():
            e, file = cls.get_by_id(file[0].parent_id)
            if not e:
                raise RuntimeError("Database error (File retrieval)!")
        else:
            raise RuntimeError("Database error (File doesn't exist)!")
        return file

    @classmethod
    @DB.connection_context()
    def get_all_parent_folders(cls, start_id):
        parent_folders = []
        current_id = start_id
        while current_id:
            e, file = cls.get_by_id(current_id)
            if file.parent_id != file.id and e:
                parent_folders.append(file)
                current_id = file.parent_id
            else:
                parent_folders.append(file)
                break
        return parent_folders

    @classmethod
    @DB.connection_context()
    def insert(cls, file):
        if not cls.save(**file):
            raise RuntimeError("Database error (File)!")
        e, file = cls.get_by_id(file["id"])
        if not e:
            raise RuntimeError("Database error (File retrieval)!")
        return file

    @classmethod
    @DB.connection_context()
    def delete(cls, file):
        return cls.delete_by_id(file.id)

    @classmethod
    @DB.connection_context()
    def delete_by_pf_id(cls, folder_id):
        return cls.model.delete().where(cls.model.parent_id == folder_id).execute()

    @classmethod
    @DB.connection_context()
    def delete_folder_by_pf_id(cls, user_id, folder_id):
        try:
            files = cls.model.select().where((cls.model.tenant_id == user_id)
                                             & (cls.model.parent_id == folder_id))
            for file in files:
                cls.delete_folder_by_pf_id(user_id, file.id)
            return cls.model.delete().where((cls.model.tenant_id == user_id)
                                            & (cls.model.id == folder_id)).execute(),
        except Exception as e:
            print(e)
            raise RuntimeError("Database error (File retrieval)!")

    @classmethod
    @DB.connection_context()
    def get_file_count(cls, tenant_id):
        files = cls.model.select(cls.model.id).where(cls.model.tenant_id == tenant_id)
        return len(files)

    @classmethod
    @DB.connection_context()
    def get_folder_size(cls, folder_id):
        size = 0

        def dfs(parent_id):
            nonlocal size
            for f in cls.model.select(*[cls.model.id, cls.model.size, cls.model.type]).where(
                    cls.model.parent_id == parent_id, cls.model.id != parent_id):
                size += f.size
                if f.type == FileType.FOLDER.value:
                    dfs(f.id)

        dfs(folder_id)
        return size

    @classmethod
    @DB.connection_context()
    def add_file_from_kb(cls, doc, kb_folder_id, tenant_id):
        for _ in File2DocumentService.get_by_document_id(doc["id"]): return
        file = {
            "id": get_uuid(),
            "parent_id": kb_folder_id,
            "tenant_id": tenant_id,
            "created_by": tenant_id,
            "name": doc["name"],
            "type": doc["type"],
            "size": doc["size"],
            "location": doc["location"],
            "source_type": FileSource.KNOWLEDGEBASE
        }
        cls.save(**file)
        File2DocumentService.save(**{"id": get_uuid(), "file_id": file["id"], "document_id": doc["id"]})