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
#  limitations under the License
#

from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService

from flask import request
from flask_login import login_required, current_user
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request
from api.utils import get_uuid
from api.db import FileType
from api.db.services.document_service import DocumentService
from api import settings
from api.utils.api_utils import get_json_result


@manager.route('/convert', methods=['POST'])  # noqa: F821
@login_required
@validate_request("file_ids", "kb_ids")
def convert():
    req = request.json
    kb_ids = req["kb_ids"]
    file_ids = req["file_ids"]
    file2documents = []

    try:
        files = FileService.get_by_ids(file_ids)
        files_set = dict({file.id: file for file in files})
        for file_id in file_ids:
            file = files_set[file_id]
            if not file:
                return get_data_error_result(message="File not found!")
                
            # 首先处理文件自身（包括文件夹）的关联
            # 删除现有关联
            informs = File2DocumentService.get_by_file_id(file_id)
            for inform in informs:
                doc_id = inform.document_id
                e, doc = DocumentService.get_by_id(doc_id)
                if not e:
                    return get_data_error_result(message="Document not found!")
                tenant_id = DocumentService.get_tenant_id(doc_id)
                if not tenant_id:
                    return get_data_error_result(message="Tenant not found!")
                if not DocumentService.remove_document(doc, tenant_id):
                    return get_data_error_result(
                        message="Database error (Document removal)!")
            File2DocumentService.delete_by_file_id(file_id)
            
            # 为文件本身（包括文件夹）创建关联
            for kb_id in kb_ids:
                e, kb = KnowledgebaseService.get_by_id(kb_id)
                if not e:
                    return get_data_error_result(
                        message="Can't find this knowledgebase!")
                
                doc = DocumentService.insert({
                    "id": get_uuid(),
                    "kb_id": kb.id,
                    "parser_id": FileService.get_parser(file.type, file.name, kb.parser_id),
                    "parser_config": kb.parser_config,
                    "created_by": current_user.id,
                    "type": file.type,
                    "name": file.name,
                    "location": file.location,
                    "size": file.size
                })
                file2document = File2DocumentService.insert({
                    "id": get_uuid(),
                    "file_id": file_id,
                    "document_id": doc.id,
                })
                file2documents.append(file2document.to_json())
            
            # 如果是文件夹，递归处理内部文件
            if file.type == FileType.FOLDER.value:
                file_ids_list = FileService.get_all_innermost_file_ids(file_id, [])
                # 排除当前文件夹自身
                if file_id in file_ids_list:
                    file_ids_list.remove(file_id)
                    
                for inner_file_id in file_ids_list:
                    # 删除现有关联
                    informs = File2DocumentService.get_by_file_id(inner_file_id)
                    for inform in informs:
                        doc_id = inform.document_id
                        e, doc = DocumentService.get_by_id(doc_id)
                        if not e:
                            return get_data_error_result(message="Document not found!")
                        tenant_id = DocumentService.get_tenant_id(doc_id)
                        if not tenant_id:
                            return get_data_error_result(message="Tenant not found!")
                        if not DocumentService.remove_document(doc, tenant_id):
                            return get_data_error_result(
                                message="Database error (Document removal)!")
                    File2DocumentService.delete_by_file_id(inner_file_id)

                    # 为内部文件创建新关联
                    e, inner_file = FileService.get_by_id(inner_file_id)
                    if not e:
                        return get_data_error_result(
                            message="Can't find this file!")
                            
                    for kb_id in kb_ids:
                        e, kb = KnowledgebaseService.get_by_id(kb_id)
                        if not e:
                            return get_data_error_result(
                                message="Can't find this knowledgebase!")
                                
                        doc = DocumentService.insert({
                            "id": get_uuid(),
                            "kb_id": kb.id,
                            "parser_id": FileService.get_parser(inner_file.type, inner_file.name, kb.parser_id),
                            "parser_config": kb.parser_config,
                            "created_by": current_user.id,
                            "type": inner_file.type,
                            "name": inner_file.name,
                            "location": inner_file.location,
                            "size": inner_file.size
                        })
                        file2document = File2DocumentService.insert({
                            "id": get_uuid(),
                            "file_id": inner_file_id,
                            "document_id": doc.id,
                        })
                        file2documents.append(file2document.to_json())
                        
        return get_json_result(data=file2documents)
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['POST'])  # noqa: F821
@login_required
@validate_request("file_ids")
def rm():
    req = request.json
    file_ids = req["file_ids"]
    if not file_ids:
        return get_json_result(
            data=False, message='Lack of "Files ID"', code=settings.RetCode.ARGUMENT_ERROR)
    try:
        for file_id in file_ids:
            # 处理文件本身的关联
            informs = File2DocumentService.get_by_file_id(file_id)
            if not informs:
                continue
                
            for inform in informs:
                if not inform:
                    continue
                
                # 删除文件的文档关联
                doc_id = inform.document_id
                e, doc = DocumentService.get_by_id(doc_id)
                if not e:
                    return get_data_error_result(message="Document not found!")
                    
                tenant_id = DocumentService.get_tenant_id(doc_id)
                if not tenant_id:
                    return get_data_error_result(message="Tenant not found!")
                    
                if not DocumentService.remove_document(doc, tenant_id):
                    return get_data_error_result(
                        message="Database error (Document removal)!")
                        
            File2DocumentService.delete_by_file_id(file_id)
            
            # 检查是否是文件夹，如果是则处理内部文件
            e, file = FileService.get_by_id(file_id)
            if not e:
                continue
                
            if file.type == FileType.FOLDER.value:
                # 获取文件夹内所有文件ID
                inner_file_ids = FileService.get_all_innermost_file_ids(file_id, [])
                if file_id in inner_file_ids:
                    inner_file_ids.remove(file_id)  # 排除文件夹自身
                
                # 处理内部每个文件
                for inner_file_id in inner_file_ids:
                    inner_informs = File2DocumentService.get_by_file_id(inner_file_id)
                    if not inner_informs:
                        continue
                        
                    for inner_inform in inner_informs:
                        if not inner_inform:
                            continue
                            
                        # 删除内部文件的文档关联
                        inner_doc_id = inner_inform.document_id
                        e, inner_doc = DocumentService.get_by_id(inner_doc_id)
                        if not e:
                            return get_data_error_result(message="Document not found!")
                            
                        inner_tenant_id = DocumentService.get_tenant_id(inner_doc_id)
                        if not inner_tenant_id:
                            return get_data_error_result(message="Tenant not found!")
                            
                        if not DocumentService.remove_document(inner_doc, inner_tenant_id):
                            return get_data_error_result(
                                message="Database error (Document removal)!")
                                
                    File2DocumentService.delete_by_file_id(inner_file_id)
                    
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)
