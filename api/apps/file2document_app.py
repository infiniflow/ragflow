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
        
        # 递归处理文件和文件夹的关联知识库
        def process_file(file_id, file_obj, parent_kb_ids=None):
            nonlocal file2documents
            
            # 如果传入了父文件夹的知识库IDs，使用它们；否则使用请求中的kb_ids
            current_kb_ids = parent_kb_ids if parent_kb_ids is not None else kb_ids
            
            # 获取文件的现有关联
            informs = File2DocumentService.get_by_file_id(file_id)
            
            # 处理文档删除
            for inform in informs:
                doc_id = inform.document_id
                # 如果有文档ID，删除对应的文档
                if doc_id is not None:
                    e, doc = DocumentService.get_by_id(doc_id)
                    if not e:
                        return get_data_error_result(message="Document not found!")
                    
                    # 获取文档所属租户ID
                    tenant_id = DocumentService.get_tenant_id(doc_id)
                    if not tenant_id:
                        return get_data_error_result(message="Tenant not found!")
                    
                    # 删除旧知识库中的文档
                    if not DocumentService.remove_document(doc, tenant_id):
                        return get_data_error_result(message="Database error (Document removal)!")
            
            # 删除所有现有关联
            File2DocumentService.delete_by_file_id(file_id)
            
            # 如果是文件夹，处理自身关联并递归处理子文件
            if file_obj.type == FileType.FOLDER.value:
                # 为文件夹建立新的关联关系
                folder_file2documents = []
                for kb_id in current_kb_ids:
                    e, kb = KnowledgebaseService.get_by_id(kb_id)
                    if not e:
                        return get_data_error_result(message="Knowledgebase not found!")
                    # 只记录文件夹和知识库的关联关系
                    file2doc = {
                        "id": get_uuid(),
                        "file_id": file_id,
                        "kb_id": kb_id,
                        "document_id": None  # 文件夹不创建文档记录
                    }
                    File2DocumentService.insert(file2doc)
                    folder_file2documents.append(file2doc)
                
                # 递归处理子文件和子文件夹
                query = FileService.model.select().where(FileService.model.parent_id == file_id)
                for child_file in query:
                    # 递归处理子文件/子文件夹，传递当前文件夹的知识库ID列表
                    process_file(child_file.id, child_file, current_kb_ids)
                
                file2documents.extend(folder_file2documents)
                return None
            
            # 处理普通文件的关联
            file_file2documents = []
            for kb_id in current_kb_ids:
                e, kb = KnowledgebaseService.get_by_id(kb_id)
                if not e:
                    return get_data_error_result(message="Knowledgebase not found!")

                doc = {
                    "id": get_uuid(),
                    "kb_id": kb.id,
                    "parser_id": kb.parser_id,
                    "parser_config": kb.parser_config,
                    "created_by": current_user.id,
                    "type": file_obj.type,
                    "name": file_obj.name,
                    "location": file_obj.location,
                    "size": file_obj.size,
                    "thumbnail": getattr(file_obj, 'thumbnail', None),
                }
                doc = DocumentService.insert(doc)
                file2document = {
                    "id": get_uuid(),
                    "file_id": file_id,
                    "kb_id": kb_id,
                    "document_id": doc.id
                }
                File2DocumentService.insert(file2document)
                file_file2documents.append(file2document)
            
            file2documents.extend(file_file2documents)
            return None
        
        # 处理每个顶级文件/文件夹
        for file_id in file_ids:
            file_obj = files_set[file_id]
            if not file_obj:
                return get_data_error_result(message="File not found!")
            
            result = process_file(file_id, file_obj)
            if result is not None:  # 如果返回了错误结果
                return result

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
                # 文件夹关联知识库时document_id为null，跳过文档删除
                if doc_id is None:
                    continue
                    
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
                        # 文件夹关联知识库时document_id为null，跳过文档删除
                        if inner_doc_id is None:
                            continue
                            
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
