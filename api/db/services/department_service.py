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
from peewee import fn

from api.db.db_models import Department, DepartmentUser, User
from api.utils import get_uuid
from api.db.services.base_service import BaseModelService


class DepartmentService(BaseModelService):
    """Service for department operations"""

    model = Department

    @classmethod
    def get_tenant_departments(cls, tenant_id):
        """Get all departments of a tenant"""
        return cls.model.query(tenant_id=tenant_id, status="1")

    @classmethod
    def get_department_by_id(cls, department_id):
        """Get a department by ID"""
        departments = cls.model.query(id=department_id, status="1")
        return departments[0] if departments else None

    @classmethod
    def create_department(cls, tenant_id, name, description=None, parent_id=None):
        """Create a new department"""
        department_id = get_uuid()
        department = {
            "id": department_id,
            "tenant_id": tenant_id,
            "name": name,
            "description": description,
            "parent_id": parent_id,
            "status": "1"
        }
        cls.insert(**department)
        return department_id

    @classmethod
    def update_department(cls, department_id, **kwargs):
        """Update a department"""
        return cls.update_by_id(department_id, kwargs)

    @classmethod
    def delete_department(cls, department_id):
        """Delete a department (mark as deleted)"""
        return cls.update_by_id(department_id, {"status": "0"})

    @classmethod
    def get_department_users(cls, department_id):
        """Get all users in a department"""
        query = (User
                 .select(User)
                 .join(DepartmentUser, on=(User.id == DepartmentUser.user_id))
                 .where(
                     (DepartmentUser.department_id == department_id) &
                     (DepartmentUser.status == "1") &
                     (User.status == "1")
                 ))
        return list(query)

    @classmethod
    def get_department_with_user_count(cls, tenant_id):
        """Get departments with user count for a tenant"""
        departments = cls.get_tenant_departments(tenant_id)
        result = []
        
        for dept in departments:
            dept_dict = dept.to_dict()
            dept_dict['member_count'] = DepartmentUserService.count_users(dept.id)
            result.append(dept_dict)
            
        return result


class DepartmentUserService(BaseModelService):
    """Service for department user operations"""

    model = DepartmentUser

    @classmethod
    def add_user_to_department(cls, department_id, user_id):
        """Add a user to a department"""
        # Check if already exists
        relations = cls.model.query(department_id=department_id, user_id=user_id, status="1")
        if relations:
            return relations[0].id

        relation_id = get_uuid()
        relation = {
            "id": relation_id,
            "department_id": department_id,
            "user_id": user_id,
            "status": "1"
        }
        cls.insert(**relation)
        return relation_id

    @classmethod
    def add_users_to_department(cls, department_id, user_ids):
        """Add multiple users to a department"""
        added_ids = []
        for user_id in user_ids:
            relation_id = cls.add_user_to_department(department_id, user_id)
            added_ids.append(relation_id)
        return added_ids

    @classmethod
    def remove_user_from_department(cls, department_id, user_id):
        """Remove a user from a department (mark as deleted)"""
        relations = cls.model.query(department_id=department_id, user_id=user_id, status="1")
        if not relations:
            return False
        return cls.update_by_id(relations[0].id, {"status": "0"})

    @classmethod
    def get_user_departments(cls, user_id):
        """Get all departments of a user"""
        query = (Department
                 .select(Department)
                 .join(DepartmentUser, on=(Department.id == DepartmentUser.department_id))
                 .where(
                     (DepartmentUser.user_id == user_id) &
                     (DepartmentUser.status == "1") &
                     (Department.status == "1")
                 ))
        return list(query)

    @classmethod
    def count_users(cls, department_id):
        """Count the number of users in a department"""
        return cls.model.select(fn.COUNT(cls.model.id).alias('count')).where(
            (cls.model.department_id == department_id) &
            (cls.model.status == "1")
        ).scalar()