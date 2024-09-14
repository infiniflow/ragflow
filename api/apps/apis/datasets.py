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

from api.apps import auth
from api.apps.services import dataset_service
from api.utils.api_utils import server_error_response


@manager.post('')
@manager.input(dataset_service.CreateDatasetReq, location='json')
@manager.auth_required(auth)
def create_dataset(json_data):
    try:
        tenant_id = auth.current_user.id
        return dataset_service.create_dataset(tenant_id, json_data)
    except Exception as e:
        return server_error_response(e)


@manager.put('')
@manager.input(dataset_service.UpdateDatasetReq, location='json')
@manager.auth_required(auth)
def update_dataset(json_data):
    try:
        tenant_id = auth.current_user.id
        return dataset_service.update_dataset(tenant_id, json_data)
    except Exception as e:
        return server_error_response(e)


@manager.get('/<string:kb_id>')
@manager.auth_required(auth)
def get_dataset_by_id(kb_id):
    try:
        tenant_id = auth.current_user.id
        return dataset_service.get_dataset_by_id(tenant_id, kb_id)
    except Exception as e:
        return server_error_response(e)


@manager.get('/search')
@manager.input(dataset_service.SearchDatasetReq, location='query')
@manager.auth_required(auth)
def get_dataset_by_name(query_data):
    try:
        tenant_id = auth.current_user.id
        return dataset_service.get_dataset_by_name(tenant_id, query_data["name"])
    except Exception as e:
        return server_error_response(e)


@manager.get('')
@manager.input(dataset_service.QueryDatasetReq, location='query')
@manager.auth_required(auth)
def get_all_datasets(query_data):
    try:
        tenant_id = auth.current_user.id
        return dataset_service.get_all_datasets(
            tenant_id,
            query_data['page'],
            query_data['page_size'],
            query_data['orderby'],
            query_data['desc'],
        )
    except Exception as e:
        return server_error_response(e)


@manager.delete('/<string:kb_id>')
@manager.auth_required(auth)
def delete_dataset(kb_id):
    try:
        tenant_id = auth.current_user.id
        return dataset_service.delete_dataset(tenant_id, kb_id)
    except Exception as e:
        return server_error_response(e)
