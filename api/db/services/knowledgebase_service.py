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
from datetime import datetime

from peewee import fn, JOIN

from api.db import TenantPermission
from api.db.db_models import DB, Document, Knowledgebase, User, UserTenant, UserCanvas
from api.db.services.common_service import CommonService
from common.time_utils import current_timestamp, datetime_format
from api.db.services import duplicate_name
from api.db.services.user_service import TenantService
from common.misc_utils import get_uuid
from common.constants import StatusEnum
from api.constants import DATASET_NAME_LIMIT
from api.utils.api_utils import get_parser_config, get_data_error_result


class KnowledgebaseService(CommonService):
    """Service class for managing dataset operations.

    This class extends CommonService to provide specialized functionality for dataset
    management, including document parsing status tracking, access control, and configuration
    management. It handles operations such as listing, creating, updating, and deleting
    knowledge bases, as well as managing their associated documents and permissions.

    The class implements a comprehensive set of methods for:
    - Document parsing status verification
    - Knowledge base access control
    - Parser configuration management
    - Tenant-based dataset organization

    Attributes:
        model: The Knowledgebase model class for database operations.
    """
    model = Knowledgebase

    @classmethod
    @DB.connection_context()
    def accessible4deletion(cls, kb_id, user_id):
        """Check if a dataset can be deleted by a specific user.

        This method verifies whether a user has permission to delete a dataset
        by checking if they are the creator of that dataset.

        Args:
            kb_id (str): The unique identifier of the dataset to check.
            user_id (str): The unique identifier of the user attempting the deletion.

        Returns:
            bool: True if the user has permission to delete the dataset,
                  False if the user doesn't have permission or the dataset doesn't exist.

        Example:
            >>> KnowledgebaseService.accessible4deletion("kb123", "user456")
            True

        Note:
            - This method only checks creator permissions
            - A return value of False can mean either:
                1. The dataset doesn't exist
                2. The user is not the creator of the dataset
        """
        # Check if a dataset can be deleted by a user
        docs = cls.model.select(
            cls.model.id).where(cls.model.id == kb_id, cls.model.created_by == user_id).paginate(0, 1)
        docs = docs.dicts()
        if not docs:
            return False
        return True

    @classmethod
    @DB.connection_context()
    def is_parsed_done(cls, kb_id):
        # Check if all documents in the dataset have completed parsing
        #
        # Args:
        #     kb_id: Knowledge base ID
        #
        # Returns:
        #     If all documents are parsed successfully, returns (True, None)
        #     If any document is not fully parsed, returns (False, error_message)
        from common.constants import TaskStatus
        from api.db.services.document_service import DocumentService

        # Get dataset information
        kbs = cls.query(id=kb_id)
        if not kbs:
            return False, "Knowledge base not found"
        kb = kbs[0]

        # Get all documents in the dataset
        docs, _ = DocumentService.get_by_kb_id(kb_id, 1, 1000, "create_time", True, "", [], [])

        # Check parsing status of each document
        for doc in docs:
            # If document is being parsed, don't allow chat creation
            if doc['run'] == TaskStatus.RUNNING.value or doc['run'] == TaskStatus.CANCEL.value or doc['run'] == TaskStatus.FAIL.value:
                return False, f"Document '{doc['name']}' in dataset '{kb.name}' is still being parsed. Please wait until all documents are parsed before starting a chat."
            # If document is not yet parsed and has no chunks, don't allow chat creation
            if doc['run'] == TaskStatus.UNSTART.value and doc['chunk_num'] == 0:
                return False, f"Document '{doc['name']}' in dataset '{kb.name}' has not been parsed yet. Please parse all documents before starting a chat."

        return True, None

    @classmethod
    @DB.connection_context()
    def list_documents_by_ids(cls, kb_ids):
        # Get document IDs associated with given dataset IDs
        # Args:
        #     kb_ids: List of dataset IDs
        # Returns:
        #     List of document IDs
        doc_ids = cls.model.select(Document.id.alias("document_id")).join(Document, on=(cls.model.id == Document.kb_id)).where(
            cls.model.id.in_(kb_ids)
        )
        doc_ids = list(doc_ids.dicts())
        doc_ids = [doc["document_id"] for doc in doc_ids]
        return doc_ids

    @classmethod
    @DB.connection_context()
    def get_by_tenant_ids(cls, joined_tenant_ids, user_id,
                          page_number, items_per_page,
                          orderby, desc, keywords,
                          parser_id=None
                          ):
        # Get knowledge bases by tenant IDs with pagination and filtering
        # Args:
        #     joined_tenant_ids: List of tenant IDs
        #     user_id: Current user ID
        #     page_number: Page number for pagination
        #     items_per_page: Number of items per page
        #     orderby: Field to order by
        #     desc: Boolean indicating descending order
        #     keywords: Search keywords
        #     parser_id: Optional parser ID filter
        # Returns:
        #     Tuple of (knowledge_base_list, total_count)
        fields = [
            cls.model.id,
            cls.model.avatar,
            cls.model.name,
            cls.model.language,
            cls.model.description,
            cls.model.tenant_id,
            cls.model.permission,
            cls.model.doc_num,
            cls.model.token_num,
            cls.model.chunk_num,
            cls.model.parser_id,
            cls.model.embd_id,
            User.nickname,
            User.avatar.alias('tenant_avatar'),
            cls.model.update_time
        ]
        if keywords:
            kbs = cls.model.select(*fields).join(User, on=(cls.model.tenant_id == User.id)).where(
                ((cls.model.tenant_id.in_(joined_tenant_ids) & (cls.model.permission ==
                                                                TenantPermission.TEAM.value)) | (
                    cls.model.tenant_id == user_id))
                & (cls.model.status == StatusEnum.VALID.value),
                (fn.LOWER(cls.model.name).contains(keywords.lower()))
            )
        else:
            kbs = cls.model.select(*fields).join(User, on=(cls.model.tenant_id == User.id)).where(
                ((cls.model.tenant_id.in_(joined_tenant_ids) & (cls.model.permission ==
                                                                TenantPermission.TEAM.value)) | (
                    cls.model.tenant_id == user_id))
                & (cls.model.status == StatusEnum.VALID.value)
            )
        if parser_id:
            kbs = kbs.where(cls.model.parser_id == parser_id)
        if desc:
            kbs = kbs.order_by(cls.model.getter_by(orderby).desc())
        else:
            kbs = kbs.order_by(cls.model.getter_by(orderby).asc())

        count = kbs.count()

        if page_number and items_per_page:
            kbs = kbs.paginate(page_number, items_per_page)

        return list(kbs.dicts()), count

    @classmethod
    @DB.connection_context()
    def get_all_kb_by_tenant_ids(cls, tenant_ids, user_id):
        # will get all permitted kb, be cautious.
        fields = [
            cls.model.name,
            cls.model.avatar,
            cls.model.language,
            cls.model.permission,
            cls.model.doc_num,
            cls.model.token_num,
            cls.model.chunk_num,
            cls.model.status,
            cls.model.create_date,
            cls.model.update_date
        ]
        # find team kb and owned kb
        kbs = cls.model.select(*fields).where(
            (cls.model.tenant_id.in_(tenant_ids) & (cls.model.permission ==TenantPermission.TEAM.value)) | (
                cls.model.tenant_id == user_id
            )
        )
        # sort by create_time asc
        kbs.order_by(cls.model.create_time.asc())
        # maybe cause slow query by deep paginate, optimize later.
        offset, limit = 0, 50
        res = []
        while True:
            kb_batch = kbs.offset(offset).limit(limit)
            _temp = list(kb_batch.dicts())
            if not _temp:
                break
            res.extend(_temp)
            offset += limit
        return res

    @classmethod
    @DB.connection_context()
    def get_kb_ids(cls, tenant_id):
        # Get all dataset IDs for a tenant
        # Args:
        #     tenant_id: Tenant ID
        # Returns:
        #     List of dataset IDs
        fields = [
            cls.model.id,
        ]
        kbs = cls.model.select(*fields).where(cls.model.tenant_id == tenant_id)
        kb_ids = [kb.id for kb in kbs]
        return kb_ids

    @classmethod
    @DB.connection_context()
    def get_detail(cls, kb_id):
        # Get detailed information about a dataset
        # Args:
        #     kb_id: Knowledge base ID
        # Returns:
        #     Dictionary containing dataset details
        fields = [
            cls.model.id,
            cls.model.embd_id,
            cls.model.avatar,
            cls.model.name,
            cls.model.language,
            cls.model.description,
            cls.model.permission,
            cls.model.doc_num,
            cls.model.token_num,
            cls.model.chunk_num,
            cls.model.parser_id,
            cls.model.pipeline_id,
            UserCanvas.title.alias("pipeline_name"),
            UserCanvas.avatar.alias("pipeline_avatar"),
            cls.model.parser_config,
            cls.model.pagerank,
            cls.model.graphrag_task_id,
            cls.model.graphrag_task_finish_at,
            cls.model.raptor_task_id,
            cls.model.raptor_task_finish_at,
            cls.model.mindmap_task_id,
            cls.model.mindmap_task_finish_at,
            cls.model.create_time,
            cls.model.update_time
            ]
        kbs = cls.model.select(*fields)\
                .join(UserCanvas, on=(cls.model.pipeline_id == UserCanvas.id), join_type=JOIN.LEFT_OUTER)\
            .where(
            (cls.model.id == kb_id),
            (cls.model.status == StatusEnum.VALID.value)
        ).dicts()
        if not kbs:
            return None
        return kbs[0]

    @classmethod
    @DB.connection_context()
    def update_parser_config(cls, id, config):
        # Update parser configuration for a dataset
        # Args:
        #     id: Knowledge base ID
        #     config: New parser configuration
        e, m = cls.get_by_id(id)
        if not e:
            raise LookupError(f"dataset({id}) not found.")

        def dfs_update(old, new):
            # Deep update of nested configuration
            for k, v in new.items():
                if k not in old:
                    old[k] = v
                    continue
                if isinstance(v, dict):
                    assert isinstance(old[k], dict)
                    dfs_update(old[k], v)
                elif isinstance(v, list):
                    assert isinstance(old[k], list)
                    old[k] = list(set(old[k] + v))
                else:
                    old[k] = v

        dfs_update(m.parser_config, config)
        cls.update_by_id(id, {"parser_config": m.parser_config})

    @classmethod
    @DB.connection_context()
    def delete_field_map(cls, id):
        e, m = cls.get_by_id(id)
        if not e:
            raise LookupError(f"dataset({id}) not found.")

        m.parser_config.pop("field_map", None)
        cls.update_by_id(id, {"parser_config": m.parser_config})

    @classmethod
    @DB.connection_context()
    def get_field_map(cls, ids):
        # Get field mappings for knowledge bases
        # Args:
        #     ids: List of dataset IDs
        # Returns:
        #     Dictionary of field mappings
        conf = {}
        for k in cls.get_by_ids(ids):
            if k.parser_config and "field_map" in k.parser_config:
                conf.update(k.parser_config["field_map"])
        return conf

    @classmethod
    @DB.connection_context()
    def get_by_name(cls, kb_name, tenant_id):
        # Get dataset by name and tenant ID
        # Args:
        #     kb_name: Knowledge base name
        #     tenant_id: Tenant ID
        # Returns:
        #     Tuple of (exists, knowledge_base)
        kb = cls.model.select().where(
            (cls.model.name == kb_name)
            & (cls.model.tenant_id == tenant_id)
            & (cls.model.status == StatusEnum.VALID.value)
        )
        if kb:
            return True, kb[0]
        return False, None

    @classmethod
    @DB.connection_context()
    def get_all_ids(cls):
        # Get all dataset IDs
        # Returns:
        #     List of all dataset IDs
        return [m["id"] for m in cls.model.select(cls.model.id).dicts()]


    @classmethod
    @DB.connection_context()
    def create_with_name(
        cls,
        *,
        name: str,
        tenant_id: str,
        parser_id: str | None = None,
        **kwargs
    ):
        """Create a dataset (knowledgebase) by name with kb_app defaults.

        This encapsulates the creation logic used in kb_app.create so other callers
        (including RESTFul endpoints) can reuse the same behavior.

        Returns:
            (ok: bool, model_or_msg): On success, returns (True, Knowledgebase model instance);
                                      on failure, returns (False, error_message).
        """
        # Validate name
        if not isinstance(name, str):
            return False, get_data_error_result(message="Dataset name must be string.")
        dataset_name = name.strip()
        if dataset_name == "":
            return False, get_data_error_result(message="Dataset name can't be empty.")
        if len(dataset_name.encode("utf-8")) > DATASET_NAME_LIMIT:
            return False, get_data_error_result(message=f"Dataset name length is {len(dataset_name)} which is large than {DATASET_NAME_LIMIT}")

        # Deduplicate name within tenant
        dataset_name = duplicate_name(
            cls.query,
            name=dataset_name,
            tenant_id=tenant_id,
            status=StatusEnum.VALID.value,
        )

        # Verify tenant exists
        ok, _t = TenantService.get_by_id(tenant_id)
        if not ok:
            return False, get_data_error_result(message="Tenant not found.")

        # Build payload
        kb_id = get_uuid()
        payload = {
            "id": kb_id,
            "name": dataset_name,
            "tenant_id": tenant_id,
            "created_by": tenant_id,
            "parser_id": (parser_id or "naive"),
            **kwargs # Includes optional fields such as description, language, permission, avatar, parser_config, etc.
        }

        # Update parser_config (always override with validated default/merged config)
        payload["parser_config"] = get_parser_config(parser_id, kwargs.get("parser_config"))
        payload["parser_config"]["llm_id"] = _t.llm_id

        return True, payload


    @classmethod
    @DB.connection_context()
    def get_list(cls, joined_tenant_ids, user_id,
                 page_number, items_per_page, orderby, desc, id, name):
        # Get list of knowledge bases with filtering and pagination
        # Args:
        #     joined_tenant_ids: List of tenant IDs
        #     user_id: Current user ID
        #     page_number: Page number for pagination
        #     items_per_page: Number of items per page
        #     orderby: Field to order by
        #     desc: Boolean indicating descending order
        #     id: Optional ID filter
        #     name: Optional name filter
        # Returns:
        #     List of knowledge bases
        #     Total count of knowledge bases
        kbs = cls.model.select()
        if id:
            kbs = kbs.where(cls.model.id == id)
        if name:
            kbs = kbs.where(cls.model.name == name)
        kbs = kbs.where(
            ((cls.model.tenant_id.in_(joined_tenant_ids) & (cls.model.permission ==
                                                            TenantPermission.TEAM.value)) | (
                cls.model.tenant_id == user_id))
            & (cls.model.status == StatusEnum.VALID.value)
        )

        if desc:
            kbs = kbs.order_by(cls.model.getter_by(orderby).desc())
        else:
            kbs = kbs.order_by(cls.model.getter_by(orderby).asc())

        total = kbs.count()
        kbs = kbs.paginate(page_number, items_per_page)

        return list(kbs.dicts()), total

    @classmethod
    @DB.connection_context()
    def accessible(cls, kb_id, user_id):
        # Check if a dataset is accessible by a user
        # Args:
        #     kb_id: Knowledge base ID
        #     user_id: User ID
        # Returns:
        #     Boolean indicating accessibility
        docs = cls.model.select(
            cls.model.id).join(UserTenant, on=(UserTenant.tenant_id == Knowledgebase.tenant_id)
                               ).where(cls.model.id == kb_id, UserTenant.user_id == user_id).paginate(0, 1)
        docs = docs.dicts()
        if not docs:
            return False
        return True

    @classmethod
    @DB.connection_context()
    def get_kb_by_id(cls, kb_id, user_id):
        # Get dataset by ID and user ID
        # Args:
        #     kb_id: Knowledge base ID
        #     user_id: User ID
        # Returns:
        #     List containing dataset information
        kbs = cls.model.select().join(UserTenant, on=(UserTenant.tenant_id == Knowledgebase.tenant_id)
                                      ).where(cls.model.id == kb_id, UserTenant.user_id == user_id).paginate(0, 1)
        kbs = kbs.dicts()
        return list(kbs)

    @classmethod
    @DB.connection_context()
    def get_kb_by_name(cls, kb_name, user_id):
        # Get dataset by name and user ID
        # Args:
        #     kb_name: Knowledge base name
        #     user_id: User ID
        # Returns:
        #     List containing dataset information
        kbs = cls.model.select().join(UserTenant, on=(UserTenant.tenant_id == Knowledgebase.tenant_id)
                                      ).where(cls.model.name == kb_name, UserTenant.user_id == user_id).paginate(0, 1)
        kbs = kbs.dicts()
        return list(kbs)

    @classmethod
    @DB.connection_context()
    def atomic_increase_doc_num_by_id(cls, kb_id):
        data = {}
        data["update_time"] = current_timestamp()
        data["update_date"] = datetime_format(datetime.now())
        data["doc_num"] = cls.model.doc_num + 1
        num = cls.model.update(data).where(cls.model.id == kb_id).execute()
        return num

    @classmethod
    @DB.connection_context()
    def update_document_number_in_init(cls, kb_id, doc_num):
        """
        Only use this function when init system
        """
        ok, kb = cls.get_by_id(kb_id)
        if not ok:
            return
        kb.doc_num = doc_num

        dirty_fields = kb.dirty_fields
        if cls.model._meta.combined.get("update_time") in dirty_fields:
            dirty_fields.remove(cls.model._meta.combined["update_time"])

        if cls.model._meta.combined.get("update_date") in dirty_fields:
            dirty_fields.remove(cls.model._meta.combined["update_date"])

        try:
            kb.save(only=dirty_fields)
        except ValueError as e:
            if str(e) == "no data to save!":
                pass # that's OK
            else:
                raise e

    @classmethod
    @DB.connection_context()
    def decrease_document_num_in_delete(cls, kb_id, doc_num_info: dict):
        kb_row = cls.model.get_by_id(kb_id)
        if not kb_row:
            raise RuntimeError(f"kb_id {kb_id} does not exist")
        update_dict = {
            'doc_num': kb_row.doc_num - doc_num_info['doc_num'],
            'chunk_num': kb_row.chunk_num - doc_num_info['chunk_num'],
            'token_num': kb_row.token_num - doc_num_info['token_num'],
            'update_time': current_timestamp(),
            'update_date': datetime_format(datetime.now())
        }
        return cls.model.update(update_dict).where(cls.model.id == kb_id).execute()
