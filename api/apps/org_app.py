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
import logging
from flask import request, Blueprint
from flask_login import login_required, current_user

from api.utils.api_utils import (
    get_json_result,
    validate_request,
    server_error_response,
    get_data_error_result,
)
from api.db.services.user_service import TenantService
from api.db.services.department_service import DepartmentService, DepartmentUserService
from api.db.services.group_service import GroupService, GroupUserService
from api.utils import get_uuid

# 创建Blueprint对象
org_manager = Blueprint("org_manager", __name__)


# 部门相关API
@org_manager.route("/tenant/<tenant_id>/department/list", methods=["GET"])
@login_required
def list_departments(tenant_id):
    """
    获取租户的所有部门列表
    ---
    tags:
      - Department
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: tenant_id
        type: string
        required: true
        description: 租户ID
    responses:
      200:
        description: 部门列表
        schema:
          type: object
    """
    try:
        # 获取包含用户数量的部门列表
        departments = DepartmentService.get_department_with_user_count(tenant_id)
        return get_json_result(data=departments)
    except Exception as e:
        return server_error_response(e)


@org_manager.route("/tenant/<tenant_id>/department", methods=["POST"])
@login_required
@validate_request("name")
def add_department(tenant_id):
    """
    创建新部门
    ---
    tags:
      - Department
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: tenant_id
        type: string
        required: true
        description: 租户ID
      - in: body
        name: body
        description: 部门信息
        required: true
        schema:
          type: object
          properties:
            name:
              type: string
              description: 部门名称
            description:
              type: string
              description: 部门描述
            parentId:
              type: string
              description: 上级部门ID
    responses:
      200:
        description: 创建成功
        schema:
          type: object
    """
    req = request.json
    name = req.get("name")
    description = req.get("description", "")
    parent_id = req.get("parentId")
    
    try:
        # 创建部门
        department_id = DepartmentService.create_department(
            tenant_id=tenant_id,
            name=name,
            description=description,
            parent_id=parent_id
        )
        
        return get_json_result(data={"id": department_id})
    except Exception as e:
        return server_error_response(e)


@org_manager.route("/department/<department_id>", methods=["PUT"])
@login_required
def update_department(department_id):
    """
    更新部门信息
    ---
    tags:
      - Department
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: department_id
        type: string
        required: true
        description: 部门ID
      - in: body
        name: body
        description: 部门信息
        required: true
        schema:
          type: object
          properties:
            name:
              type: string
              description: 部门名称
            description:
              type: string
              description: 部门描述
            parentId:
              type: string
              description: 上级部门ID
    responses:
      200:
        description: 更新成功
        schema:
          type: object
    """
    req = request.json
    update_dict = {}
    
    if "name" in req:
        update_dict["name"] = req["name"]
    if "description" in req:
        update_dict["description"] = req["description"]
    if "parentId" in req:
        update_dict["parent_id"] = req["parentId"]
    
    try:
        # 更新部门
        result = DepartmentService.update_department(department_id, **update_dict)
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)


@org_manager.route("/department/<department_id>", methods=["DELETE"])
@login_required
def delete_department(department_id):
    """
    删除部门
    ---
    tags:
      - Department
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: department_id
        type: string
        required: true
        description: 部门ID
    responses:
      200:
        description: 删除成功
        schema:
          type: object
    """
    try:
        # 删除部门
        result = DepartmentService.delete_department(department_id)
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)


@org_manager.route("/department/<department_id>/users", methods=["POST"])
@login_required
@validate_request("userIds")
def add_users_to_department(department_id):
    """
    添加用户到部门
    ---
    tags:
      - Department
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: department_id
        type: string
        required: true
        description: 部门ID
      - in: body
        name: body
        description: 用户信息
        required: true
        schema:
          type: object
          properties:
            userIds:
              type: array
              items:
                type: string
              description: 用户ID列表
    responses:
      200:
        description: 添加成功
        schema:
          type: object
    """
    req = request.json
    user_ids = req.get("userIds", [])
    
    try:
        # 添加用户到部门
        result = DepartmentUserService.add_users_to_department(department_id, user_ids)
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)


@org_manager.route("/department/<department_id>/user/<user_id>", methods=["DELETE"])
@login_required
def remove_user_from_department(department_id, user_id):
    """
    从部门中移除用户
    ---
    tags:
      - Department
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: department_id
        type: string
        required: true
        description: 部门ID
      - in: path
        name: user_id
        type: string
        required: true
        description: 用户ID
    responses:
      200:
        description: 移除成功
        schema:
          type: object
    """
    try:
        # 从部门移除用户
        result = DepartmentUserService.remove_user_from_department(department_id, user_id)
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)


# 群组相关API
@org_manager.route("/tenant/<tenant_id>/group/list", methods=["GET"])
@login_required
def list_groups(tenant_id):
    """
    获取租户的所有群组列表
    ---
    tags:
      - Group
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: tenant_id
        type: string
        required: true
        description: 租户ID
    responses:
      200:
        description: 群组列表
        schema:
          type: object
    """
    try:
        # 获取包含用户数量的群组列表
        groups = GroupService.get_groups_with_user_count(tenant_id)
        return get_json_result(data=groups)
    except Exception as e:
        return server_error_response(e)


@org_manager.route("/tenant/<tenant_id>/group", methods=["POST"])
@login_required
@validate_request("name")
def add_group(tenant_id):
    """
    创建新群组
    ---
    tags:
      - Group
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: tenant_id
        type: string
        required: true
        description: 租户ID
      - in: body
        name: body
        description: 群组信息
        required: true
        schema:
          type: object
          properties:
            name:
              type: string
              description: 群组名称
            description:
              type: string
              description: 群组描述
    responses:
      200:
        description: 创建成功
        schema:
          type: object
    """
    req = request.json
    name = req.get("name")
    description = req.get("description", "")
    
    try:
        # 创建群组
        group_id = GroupService.create_group(
            tenant_id=tenant_id,
            name=name,
            description=description
        )
        
        return get_json_result(data={"id": group_id})
    except Exception as e:
        return server_error_response(e)


@org_manager.route("/group/<group_id>", methods=["PUT"])
@login_required
def update_group(group_id):
    """
    更新群组信息
    ---
    tags:
      - Group
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: group_id
        type: string
        required: true
        description: 群组ID
      - in: body
        name: body
        description: 群组信息
        required: true
        schema:
          type: object
          properties:
            name:
              type: string
              description: 群组名称
            description:
              type: string
              description: 群组描述
    responses:
      200:
        description: 更新成功
        schema:
          type: object
    """
    req = request.json
    update_dict = {}
    
    if "name" in req:
        update_dict["name"] = req["name"]
    if "description" in req:
        update_dict["description"] = req["description"]
    
    try:
        # 更新群组
        result = GroupService.update_group(group_id, **update_dict)
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)


@org_manager.route("/group/<group_id>", methods=["DELETE"])
@login_required
def delete_group(group_id):
    """
    删除群组
    ---
    tags:
      - Group
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: group_id
        type: string
        required: true
        description: 群组ID
    responses:
      200:
        description: 删除成功
        schema:
          type: object
    """
    try:
        # 删除群组
        result = GroupService.delete_group(group_id)
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)


@org_manager.route("/group/<group_id>/users", methods=["POST"])
@login_required
@validate_request("userIds")
def add_users_to_group(group_id):
    """
    添加用户到群组
    ---
    tags:
      - Group
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: group_id
        type: string
        required: true
        description: 群组ID
      - in: body
        name: body
        description: 用户信息
        required: true
        schema:
          type: object
          properties:
            userIds:
              type: array
              items:
                type: string
              description: 用户ID列表
    responses:
      200:
        description: 添加成功
        schema:
          type: object
    """
    req = request.json
    user_ids = req.get("userIds", [])
    
    try:
        # 添加用户到群组
        result = GroupUserService.add_users_to_group(group_id, user_ids)
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)


@org_manager.route("/group/<group_id>/user/<user_id>", methods=["DELETE"])
@login_required
def remove_user_from_group(group_id, user_id):
    """
    从群组中移除用户
    ---
    tags:
      - Group
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: group_id
        type: string
        required: true
        description: 群组ID
      - in: path
        name: user_id
        type: string
        required: true
        description: 用户ID
    responses:
      200:
        description: 移除成功
        schema:
          type: object
    """
    try:
        # 从群组移除用户
        result = GroupUserService.remove_user_from_group(group_id, user_id)
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)