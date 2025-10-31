declare module AdminService {
  export type LoginData = {
    access_token: string;
    avatar: unknown;
    color_schema: 'Bright' | 'Dark';
    create_date: string;
    create_time: number;
    email: string;
    id: string;
    is_active: '0' | '1';
    is_anonymous: '0' | '1';
    is_authenticated: '0' | '1';
    is_superuser: boolean;
    language: string;
    last_login_time: string;
    login_channel: unknown;
    nickname: string;
    password: string;
    status: '0' | '1';
    timezone: string;
    update_date: [string];
    update_time: [number];
  };

  export type ListUsersItem = {
    create_date: string;
    email: string;
    is_active: '0' | '1';
    is_superuser: boolean;
    role: string;
    nickname: string;
  };

  export type UserDetail = {
    create_date: string;
    email: string;
    is_active: '0' | '1';
    is_anonymous: '0' | '1';
    is_superuser: boolean;
    language: string;
    last_login_time: string;
    login_channel: unknown;
    status: '0' | '1';
    update_date: string;
    role: string;
  };

  export type ListUserDatasetItem = {
    chunk_num: number;
    create_date: string;
    doc_num: number;
    language: string;
    name: string;
    permission: string;
    status: '0' | '1';
    token_num: number;
    update_date: string;
  };

  export type ListUserAgentItem = {
    canvas_category: 'agent';
    permission: 'string';
    title: string;
  };

  export type ListServicesItem = {
    extra: Record<string, unknown>;
    host: string;
    id: number;
    name: string;
    port: number;
    service_type: string;
    status: 'alive' | 'timeout' | 'fail';
  };

  export type ServiceDetail = {
    service_name: string;
    status: 'alive' | 'timeout';
    message: string | Record<string, any> | Record<string, any>[];
  };

  export type PermissionData = {
    enable: boolean;
    read: boolean;
    write: boolean;
    share: boolean;
  };

  export type ListRoleItem = {
    id: string;
    role_name: string;
    description: string;
    create_date: string;
    update_date: string;
  };

  export type ListRoleItemWithPermission = ListRoleItem & {
    permissions: Record<string, PermissionData>;
  };

  export type RoleDetailWithPermission = {
    role: {
      id: string;
      name: string;
      description: string;
    };
    permissions: Record<string, PermissionData>;
  };

  export type RoleDetail = {
    id: string;
    name: string;
    descrtiption: string;
    create_date: string;
    update_date: string;
  };

  export type AssignRolePermissionsInput = Record<
    string,
    Partial<PermissionData>
  >;
  export type RevokeRolePermissionInput = AssignRolePermissionsInput;

  export type UserDetailWithPermission = {
    user: {
      id: string;
      username: string;
      role: string;
    };
    role_permissions: Record<string, PermissionData>;
  };

  export type ResourceType = {
    resource_types: string[];
  };

  export type ListWhitelistItem = {
    id: number;
    email: string;
    create_date: string;
    createt_time: number;
    update_date: string;
    update_time: number;
  };
}
