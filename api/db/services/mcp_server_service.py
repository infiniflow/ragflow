#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
from peewee import fn

from api.db.db_models import DB, MCPServer
from api.db.services.common_service import CommonService


class MCPServerService(CommonService):
    """Service class for managing MCP server related database operations.

    This class extends CommonService to provide specialized functionality for MCP server management,
    including MCP server creation, updates, and deletions.

    Attributes:
        model: The MCPServer model class for database operations.
    """

    model = MCPServer

    @classmethod
    @DB.connection_context()
    def get_servers(cls, tenant_id: str, id_list: list[str] | None, page_number, items_per_page, orderby, desc,
                    keywords):
        """Retrieve all MCP servers associated with a tenant.

        This method fetches all MCP servers for a given tenant, ordered by creation time.
        It only includes fields for list display.

        Args:
            tenant_id (str): The unique identifier of the tenant.
            id_list (list[str]): Get servers by ID list. Will ignore this condition if None.

        Returns:
            list[dict]: List of MCP server dictionaries containing MCP server details.
                       Returns None if no MCP servers are found.
        """
        fields = [
            cls.model.id,
            cls.model.name,
            cls.model.server_type,
            cls.model.url,
            cls.model.description,
            cls.model.variables,
            cls.model.create_date,
            cls.model.update_date,
        ]

        query = cls.model.select(*fields).order_by(cls.model.create_time.desc()).where(cls.model.tenant_id == tenant_id)

        if id_list:
            query = query.where(cls.model.id.in_(id_list))
        if keywords:
            query = query.where(fn.LOWER(cls.model.name).contains(keywords.lower()))
        if desc:
            query = query.order_by(cls.model.getter_by(orderby).desc())
        else:
            query = query.order_by(cls.model.getter_by(orderby).asc())
        if page_number and items_per_page:
            query = query.paginate(page_number, items_per_page)

        servers = list(query.dicts())
        if not servers:
            return None
        return servers

    @classmethod
    @DB.connection_context()
    def get_by_name_and_tenant(cls, name: str, tenant_id: str):
        try:
            mcp_server = cls.model.query(name=name, tenant_id=tenant_id)
            return bool(mcp_server), mcp_server
        except Exception:
            return False, None

    @classmethod
    @DB.connection_context()
    def delete_by_tenant_id(cls, tenant_id: str):
        return cls.model.delete().where(cls.model.tenant_id == tenant_id).execute()
