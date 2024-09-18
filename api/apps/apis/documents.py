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
from api.apps import http_token_auth
from api.apps.services import document_service
from api.utils.api_utils import server_error_response


@manager.route('/change_parser', methods=['POST'])
@manager.input(document_service.ChangeDocumentParserReq, location='json')
@manager.auth_required(http_token_auth)
def change_document_parser(json_data):
    """Change document file parser."""
    try:
        return document_service.change_document_parser(json_data)
    except Exception as e:
        return server_error_response(e)


@manager.route('/run', methods=['POST'])
@manager.input(document_service.RunParsingReq, location='json')
@manager.auth_required(http_token_auth)
def run_parsing(json_data):
    """Run parsing documents file."""
    try:
        return document_service.run_parsing(json_data)
    except Exception as e:
        return server_error_response(e)


@manager.post('/upload')
@manager.input(document_service.UploadDocumentsReq, location='form_and_files')
@manager.auth_required(http_token_auth)
def upload_documents_2_dataset(form_and_files_data):
    """Upload documents file a Dataset(Knowledgebase)."""
    try:
        tenant_id = http_token_auth.current_user.id
        return document_service.upload_documents_2_dataset(form_and_files_data, tenant_id)
    except Exception as e:
        return server_error_response(e)


@manager.get('')
@manager.input(document_service.QueryDocumentsReq, location='query')
@manager.auth_required(http_token_auth)
def get_all_documents(query_data):
    """Query documents file in Dataset(Knowledgebase)."""
    try:
        tenant_id = http_token_auth.current_user.id
        return document_service.get_all_documents(query_data, tenant_id)
    except Exception as e:
        return server_error_response(e)
