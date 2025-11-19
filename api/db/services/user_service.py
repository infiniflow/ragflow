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
import hashlib
from datetime import datetime
import logging
from typing import Any, Dict, List, Optional, Tuple

import peewee
from werkzeug.security import generate_password_hash, check_password_hash

from api.db import UserTenantRole
from api.db.db_models import DB, UserTenant
from api.db.db_models import User, Tenant, Department, UserDepartment, Group, GroupUser
from api.db.services.common_service import CommonService
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp, datetime_format
from common.constants import StatusEnum
from common import settings


class UserService(CommonService):
    """Service class for managing user-related database operations.

    This class extends CommonService to provide specialized functionality for user management,
    including authentication, user creation, updates, and deletions.

    Attributes:
        model: The User model class for database operations.
    """
    model = User

    @classmethod
    @DB.connection_context()
    def query(cls, cols=None, reverse=None, order_by=None, **kwargs):
        if 'access_token' in kwargs:
            access_token = kwargs['access_token']

            # Reject empty, None, or whitespace-only access tokens
            if not access_token or not str(access_token).strip():
                logging.warning("UserService.query: Rejecting empty access_token query")
                return cls.model.select().where(cls.model.id == "INVALID_EMPTY_TOKEN")  # Returns empty result

            # Reject tokens that are too short (should be UUID, 32+ chars)
            if len(str(access_token).strip()) < 32:
                logging.warning(f"UserService.query: Rejecting short access_token query: {len(str(access_token))} chars")
                return cls.model.select().where(cls.model.id == "INVALID_SHORT_TOKEN")  # Returns empty result

            # Reject tokens that start with "INVALID_" (from logout)
            if str(access_token).startswith("INVALID_"):
                logging.warning("UserService.query: Rejecting invalidated access_token")
                return cls.model.select().where(cls.model.id == "INVALID_LOGOUT_TOKEN")  # Returns empty result

        # Call parent query method for valid requests
        return super().query(cols=cols, reverse=reverse, order_by=order_by, **kwargs)

    @classmethod
    @DB.connection_context()
    def filter_by_id(cls, user_id):
        """Retrieve a user by their ID.

        Args:
            user_id: The unique identifier of the user.

        Returns:
            User object if found, None otherwise.
        """
        try:
            user = cls.model.select().where(cls.model.id == user_id).get()
            return user
        except peewee.DoesNotExist:
            return None

    @classmethod
    @DB.connection_context()
    def query_user(cls, email, password):
        """Authenticate a user with email and password.

        Args:
            email: User's email address.
            password: User's password in plain text.

        Returns:
            User object if authentication successful, None otherwise.
        """
        user = cls.model.select().where((cls.model.email == email),
                                        (cls.model.status == StatusEnum.VALID.value)).first()
        if user and check_password_hash(str(user.password), password):
            return user
        else:
            return None

    @classmethod
    @DB.connection_context()
    def query_user_by_email(cls, email):
        users = cls.model.select().where((cls.model.email == email))
        return list(users)

    @classmethod
    @DB.connection_context()
    def save(cls, **kwargs):
        if "id" not in kwargs:
            kwargs["id"] = get_uuid()
        if "password" in kwargs:
            kwargs["password"] = generate_password_hash(
                str(kwargs["password"]))

        kwargs["create_time"] = current_timestamp()
        kwargs["create_date"] = datetime_format(datetime.now())
        kwargs["update_time"] = current_timestamp()
        kwargs["update_date"] = datetime_format(datetime.now())
        obj = cls.model(**kwargs).save(force_insert=True)
        return obj

    @classmethod
    @DB.connection_context()
    def delete_user(cls, user_ids, update_user_dict):
        with DB.atomic():
            cls.model.update({"status": 0}).where(
                cls.model.id.in_(user_ids)).execute()

    @classmethod
    @DB.connection_context()
    def update_user(cls, user_id, user_dict):
        with DB.atomic():
            if user_dict:
                user_dict["update_time"] = current_timestamp()
                user_dict["update_date"] = datetime_format(datetime.now())
                cls.model.update(user_dict).where(
                    cls.model.id == user_id).execute()

    @classmethod
    @DB.connection_context()
    def update_user_password(cls, user_id, new_password):
        with DB.atomic():
            update_dict = {
                "password": generate_password_hash(str(new_password)),
                "update_time": current_timestamp(),
                "update_date": datetime_format(datetime.now())
            }
            cls.model.update(update_dict).where(cls.model.id == user_id).execute()

    @classmethod
    @DB.connection_context()
    def is_admin(cls, user_id):
        return cls.model.select().where(
            cls.model.id == user_id,
            cls.model.is_superuser == 1).count() > 0

    @classmethod
    @DB.connection_context()
    def get_all_users(cls):
        users = cls.model.select()
        return list(users)


class TenantService(CommonService):
    """Service class for managing tenant-related database operations.

    This class extends CommonService to provide functionality for tenant management,
    including tenant information retrieval and credit management.

    Attributes:
        model: The Tenant model class for database operations.
    """
    model = Tenant

    @classmethod
    @DB.connection_context()
    def get_info_by(cls, user_id):
        fields = [
            cls.model.id.alias("tenant_id"),
            cls.model.name,
            cls.model.llm_id,
            cls.model.embd_id,
            cls.model.rerank_id,
            cls.model.asr_id,
            cls.model.img2txt_id,
            cls.model.tts_id,
            cls.model.parser_ids,
            UserTenant.role]
        return list(cls.model.select(*fields)
                    .join(UserTenant, on=((cls.model.id == UserTenant.tenant_id) & (UserTenant.user_id == user_id) & (UserTenant.status == StatusEnum.VALID.value) & (UserTenant.role == UserTenantRole.OWNER)))
                    .where(cls.model.status == StatusEnum.VALID.value).dicts())

    @classmethod
    @DB.connection_context()
    def get_joined_tenants_by_user_id(cls, user_id):
        fields = [
            cls.model.id.alias("tenant_id"),
            cls.model.name,
            cls.model.llm_id,
            cls.model.embd_id,
            cls.model.asr_id,
            cls.model.img2txt_id,
            UserTenant.role]
        return list(cls.model.select(*fields)
                    .join(UserTenant, on=((cls.model.id == UserTenant.tenant_id) & (UserTenant.user_id == user_id) & (UserTenant.status == StatusEnum.VALID.value) & (UserTenant.role == UserTenantRole.NORMAL)))
                    .where(cls.model.status == StatusEnum.VALID.value).dicts())

    @classmethod
    @DB.connection_context()
    def decrease(cls, user_id, num):
        num = cls.model.update(credit=cls.model.credit - num).where(
            cls.model.id == user_id).execute()
        if num == 0:
            raise LookupError("Tenant not found which is supposed to be there")

    @classmethod
    @DB.connection_context()
    def user_gateway(cls, tenant_id):
        hash_obj = hashlib.sha256(tenant_id.encode("utf-8"))
        return int(hash_obj.hexdigest(), 16)%len(settings.MINIO)


class UserTenantService(CommonService):
    """Service class for managing user-tenant relationship operations.

    This class extends CommonService to handle the many-to-many relationship
    between users and tenants, managing user roles and tenant memberships.

    Attributes:
        model: The UserTenant model class for database operations.
    """
    model = UserTenant

    @classmethod
    @DB.connection_context()
    def filter_by_id(cls, user_tenant_id):
        try:
            user_tenant = cls.model.select().where((cls.model.id == user_tenant_id) & (cls.model.status == StatusEnum.VALID.value)).get()
            return user_tenant
        except peewee.DoesNotExist:
            return None

    @classmethod
    @DB.connection_context()
    def save(cls, **kwargs):
        if "id" not in kwargs:
            kwargs["id"] = get_uuid()
        obj = cls.model(**kwargs).save(force_insert=True)
        return obj

    @classmethod
    @DB.connection_context()
    def get_by_tenant_id(cls, tenant_id):
        fields = [
            cls.model.id,
            cls.model.user_id,
            cls.model.status,
            cls.model.role,
            User.nickname,
            User.email,
            User.avatar,
            User.is_authenticated,
            User.is_active,
            User.is_anonymous,
            User.status,
            User.update_date,
            User.is_superuser]
        return list(cls.model.select(*fields)
                    .join(User, on=((cls.model.user_id == User.id) & (cls.model.status == StatusEnum.VALID.value) & (cls.model.role != UserTenantRole.OWNER)))
                    .where(cls.model.tenant_id == tenant_id)
                    .dicts())

    @classmethod
    @DB.connection_context()
    def get_tenants_by_user_id(cls, user_id):
        fields = [
            cls.model.tenant_id,
            cls.model.role,
            User.nickname,
            User.email,
            User.avatar,
            User.update_date
        ]
        return list(cls.model.select(*fields)
                    .join(User, on=((cls.model.tenant_id == User.id) & (UserTenant.user_id == user_id) & (UserTenant.status == StatusEnum.VALID.value)))
                    .where(cls.model.status == StatusEnum.VALID.value).dicts())

    @classmethod
    @DB.connection_context()
    def get_user_tenant_relation_by_user_id(cls, user_id):
        fields = [
            cls.model.id,
            cls.model.user_id,
            cls.model.tenant_id,
            cls.model.role
        ]
        return list(cls.model.select(*fields).where(cls.model.user_id == user_id).dicts().dicts())

    @classmethod
    @DB.connection_context()
    def get_num_members(cls, user_id: str):
        cnt_members = cls.model.select(peewee.fn.COUNT(cls.model.id)).where(cls.model.tenant_id == user_id).scalar()
        return cnt_members

    @classmethod
    @DB.connection_context()
    def filter_by_tenant_and_user_id(cls, tenant_id, user_id):
        try:
            user_tenant = cls.model.select().where(
                (cls.model.tenant_id == tenant_id) & (cls.model.status == StatusEnum.VALID.value) &
                (cls.model.user_id == user_id)
            ).first()
            return user_tenant
        except peewee.DoesNotExist:
            return None


class DepartmentService(CommonService):
    """Service class for managing department-related database operations.

    This class extends CommonService to provide functionality for department management,
    including department creation, retrieval, and tenant-based queries.

    Attributes:
        model: The Department model class for database operations.
    """
    model = Department

    @classmethod
    @DB.connection_context()
    def get_by_tenant_id(cls, tenant_id: str) -> List[Dict[str, Any]]:
        """Get all departments for a given tenant.

        Args:
            tenant_id: The tenant ID to query departments for.

        Returns:
            List of department dictionaries with all fields.
        """
        return list(cls.model.select()
                    .where(
                        (cls.model.tenant_id == tenant_id) & 
                        (cls.model.status == StatusEnum.VALID.value)
                    )
                    .order_by(cls.model.create_time.desc())
                    .dicts())

    @classmethod
    @DB.connection_context()
    def get_by_id(cls, department_id: str) -> Tuple[bool, Optional[Department]]:
        """Get a department by its ID.

        Args:
            department_id: The department ID.

        Returns:
            Tuple of (success: bool, department: Department or None).
        """
        try:
            department: Optional[Department] = cls.model.get_or_none(
                (cls.model.id == department_id) & 
                (cls.model.status == StatusEnum.VALID.value)
            )
            if department:
                return True, department
        except Exception:
            pass
        return False, None

    @classmethod
    @DB.connection_context()
    def filter_by_tenant_and_id(cls, tenant_id: str, department_id: str) -> Optional[Department]:
        """Get a department by tenant ID and department ID.

        Args:
            tenant_id: The tenant ID.
            department_id: The department ID.

        Returns:
            Department instance or None if not found.
        """
        try:
            department: Optional[Department] = cls.model.select().where(
                (cls.model.tenant_id == tenant_id) & 
                (cls.model.id == department_id) & 
                (cls.model.status == StatusEnum.VALID.value)
            ).first()
            return department
        except peewee.DoesNotExist:
            return None


class UserDepartmentService(CommonService):
    """Service class for managing user-department relationship operations.
    
    This class extends CommonService to handle the many-to-many relationship
    between users and departments.
    
    Attributes:
        model: The UserDepartment model class for database operations.
    """
    model = UserDepartment
    
    @classmethod
    @DB.connection_context()
    def save(cls, **kwargs) -> UserDepartment:
        """Save a user-department relationship.
        
        Args:
            **kwargs: UserDepartment fields (id, department_id, user_id, status).
            
        Returns:
            Created UserDepartment instance.
        """
        if "id" not in kwargs:
            kwargs["id"] = get_uuid()
        obj = cls.model(**kwargs)
        obj.save(force_insert=True)
        return obj
    
    @classmethod
    @DB.connection_context()
    def filter_by_department_and_user_id(
        cls, department_id: str, user_id: str
    ) -> Optional[UserDepartment]:
        """Get a user-department relationship by department ID and user ID.
        
        Args:
            department_id: The department ID.
            user_id: The user ID.
            
        Returns:
            UserDepartment instance or None if not found.
        """
        try:
            user_department: Optional[UserDepartment] = cls.model.select().where(
                (cls.model.department_id == department_id) &
                (cls.model.user_id == user_id)
            ).first()
            return user_department
        except peewee.DoesNotExist:
            return None
    
    @classmethod
    @DB.connection_context()
    def get_by_department_id(cls, department_id: str) -> List[Dict[str, Any]]:
        """Get all users in a department.
        
        Args:
            department_id: The department ID.
            
        Returns:
            List of user dictionaries with department relationship info.
        """
        fields = [
            cls.model.id,
            cls.model.user_id,
            cls.model.status,
            User.nickname,
            User.email,
            User.avatar,
            User.is_active,
            User.status,
        ]
        return list(
            cls.model.select(*fields)
            .join(User, on=((cls.model.user_id == User.id) & (cls.model.status == StatusEnum.VALID.value)))
            .where(cls.model.department_id == department_id)
            .dicts()
        )


class GroupService(CommonService):
    """Service class for managing group-related database operations.

    This class extends CommonService to provide functionality for group management,
    including group creation, retrieval, and tenant-based queries.

    Attributes:
        model: The Group model class for database operations.
    """
    model = Group

    @classmethod
    @DB.connection_context()
    def get_by_tenant_id(cls, tenant_id: str) -> List[Dict[str, Any]]:
        """Get all groups for a given tenant.

        Args:
            tenant_id: The tenant ID to query groups for.

        Returns:
            List of group dictionaries with all fields.
        """
        return list(cls.model.select()
                    .where(
                        (cls.model.tenant_id == tenant_id) & 
                        (cls.model.status == StatusEnum.VALID.value)
                    )
                    .order_by(cls.model.create_time.desc())
                    .dicts())

    @classmethod
    @DB.connection_context()
    def get_by_id(cls, group_id: str) -> Tuple[bool, Optional[Group]]:
        """Get a group by its ID.

        Args:
            group_id: The group ID.

        Returns:
            Tuple of (success: bool, group: Group or None).
        """
        try:
            group: Optional[Group] = cls.model.get_or_none(
                (cls.model.id == group_id) & 
                (cls.model.status == StatusEnum.VALID.value)
            )
            if group:
                return True, group
        except Exception:
            pass
        return False, None

    @classmethod
    @DB.connection_context()
    def filter_by_tenant_and_id(cls, tenant_id: str, group_id: str) -> Optional[Group]:
        """Get a group by tenant ID and group ID.

        Args:
            tenant_id: The tenant ID.
            group_id: The group ID.

        Returns:
            Group instance or None if not found.
        """
        try:
            group: Optional[Group] = cls.model.select().where(
                (cls.model.tenant_id == tenant_id) & 
                (cls.model.id == group_id) & 
                (cls.model.status == StatusEnum.VALID.value)
            ).first()
            return group
        except peewee.DoesNotExist:
            return None

    @classmethod
    @DB.connection_context()
    def get_by_tenant_and_name(cls, tenant_id: str, name: str) -> Optional[Group]:
        """Get a group by tenant ID and name.

        Args:
            tenant_id: The tenant ID.
            name: The group name.

        Returns:
            Group instance or None if not found.
        """
        try:
            group: Optional[Group] = cls.model.select().where(
                (cls.model.tenant_id == tenant_id) & 
                (cls.model.name == name) & 
                (cls.model.status == StatusEnum.VALID.value)
            ).first()
            return group
        except peewee.DoesNotExist:
            return None

    @classmethod
    @DB.connection_context()
    def save(cls, **kwargs: Any) -> Group:
        """Save a group to the database.

        Args:
            **kwargs: Group fields including id, tenant_id, name, description, created_by, status.

        Returns:
            The saved Group instance.
        """
        if "id" not in kwargs:
            kwargs["id"] = get_uuid()
        obj: Group = cls.model(**kwargs)
        obj.save(force_insert=True)
        return obj


class GroupUserService(CommonService):
    """Service class for managing user-group relationship operations.
    
    This class extends CommonService to handle the many-to-many relationship
    between users and groups.
    
    Attributes:
        model: The GroupUser model class for database operations.
    """
    model = GroupUser
    
    @classmethod
    @DB.connection_context()
    def save(cls, **kwargs: Any) -> GroupUser:
        """Save a user-group relationship.
        
        Args:
            **kwargs: GroupUser fields (id, group_id, user_id, status).
            
        Returns:
            Created GroupUser instance.
        """
        if "id" not in kwargs:
            kwargs["id"] = get_uuid()
        obj: GroupUser = cls.model(**kwargs)
        obj.save(force_insert=True)
        return obj
    
    @classmethod
    @DB.connection_context()
    def filter_by_group_and_user_id(
        cls, group_id: str, user_id: str
    ) -> Optional[GroupUser]:
        """Get a user-group relationship by group ID and user ID.
        
        Args:
            group_id: The group ID.
            user_id: The user ID.
            
        Returns:
            GroupUser instance or None if not found.
        """
        try:
            group_user: Optional[GroupUser] = cls.model.select().where(
                (cls.model.group_id == group_id) &
                (cls.model.user_id == user_id)
            ).first()
            return group_user
        except peewee.DoesNotExist:
            return None
    
    @classmethod
    @DB.connection_context()
    def get_by_group_id(cls, group_id: str) -> List[Dict[str, Any]]:
        """Get all users in a group.
        
        Args:
            group_id: The group ID.
            
        Returns:
            List of user dictionaries with group relationship info.
        """
        fields = [
            cls.model.id,
            cls.model.user_id,
            cls.model.status,
            User.nickname,
            User.email,
            User.avatar,
            User.is_active,
            User.status,
        ]
        return list(
            cls.model.select(*fields)
            .join(User, on=((cls.model.user_id == User.id) & (cls.model.status == StatusEnum.VALID.value)))
            .where(cls.model.group_id == group_id)
            .dicts()
        )
