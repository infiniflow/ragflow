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

from api.db.db_models import Group, GroupUser, User
from api.utils import get_uuid
from api.db.services.base_service import BaseModelService


class GroupService(BaseModelService):
    """Service for group operations"""

    model = Group

    @classmethod
    def get_tenant_groups(cls, tenant_id):
        """Get all groups of a tenant"""
        return cls.model.query(tenant_id=tenant_id, status="1")

    @classmethod
    def get_group_by_id(cls, group_id):
        """Get a group by ID"""
        groups = cls.model.query(id=group_id, status="1")
        return groups[0] if groups else None

    @classmethod
    def create_group(cls, tenant_id, name, description=None):
        """Create a new group"""
        group_id = get_uuid()
        group = {
            "id": group_id,
            "tenant_id": tenant_id,
            "name": name,
            "description": description,
            "status": "1"
        }
        cls.insert(**group)
        return group_id

    @classmethod
    def update_group(cls, group_id, **kwargs):
        """Update a group"""
        return cls.update_by_id(group_id, kwargs)

    @classmethod
    def delete_group(cls, group_id):
        """Delete a group (mark as deleted)"""
        return cls.update_by_id(group_id, {"status": "0"})

    @classmethod
    def get_group_users(cls, group_id):
        """Get all users in a group"""
        query = (User
                 .select(User)
                 .join(GroupUser, on=(User.id == GroupUser.user_id))
                 .where(
                     (GroupUser.group_id == group_id) &
                     (GroupUser.status == "1") &
                     (User.status == "1")
                 ))
        return list(query)

    @classmethod
    def get_groups_with_user_count(cls, tenant_id):
        """Get groups with user count for a tenant"""
        groups = cls.get_tenant_groups(tenant_id)
        result = []
        
        for group in groups:
            group_dict = group.to_dict()
            group_dict['member_count'] = GroupUserService.count_users(group.id)
            result.append(group_dict)
            
        return result


class GroupUserService(BaseModelService):
    """Service for group user operations"""

    model = GroupUser

    @classmethod
    def add_user_to_group(cls, group_id, user_id):
        """Add a user to a group"""
        # Check if already exists
        relations = cls.model.query(group_id=group_id, user_id=user_id, status="1")
        if relations:
            return relations[0].id

        relation_id = get_uuid()
        relation = {
            "id": relation_id,
            "group_id": group_id,
            "user_id": user_id,
            "status": "1"
        }
        cls.insert(**relation)
        return relation_id

    @classmethod
    def add_users_to_group(cls, group_id, user_ids):
        """Add multiple users to a group"""
        added_ids = []
        for user_id in user_ids:
            relation_id = cls.add_user_to_group(group_id, user_id)
            added_ids.append(relation_id)
        return added_ids

    @classmethod
    def remove_user_from_group(cls, group_id, user_id):
        """Remove a user from a group (mark as deleted)"""
        relations = cls.model.query(group_id=group_id, user_id=user_id, status="1")
        if not relations:
            return False
        return cls.update_by_id(relations[0].id, {"status": "0"})

    @classmethod
    def get_user_groups(cls, user_id):
        """Get all groups of a user"""
        query = (Group
                 .select(Group)
                 .join(GroupUser, on=(Group.id == GroupUser.group_id))
                 .where(
                     (GroupUser.user_id == user_id) &
                     (GroupUser.status == "1") &
                     (Group.status == "1")
                 ))
        return list(query)

    @classmethod
    def count_users(cls, group_id):
        """Count the number of users in a group"""
        return cls.model.select(fn.COUNT(cls.model.id).alias('count')).where(
            (cls.model.group_id == group_id) &
            (cls.model.status == "1")
        ).scalar()