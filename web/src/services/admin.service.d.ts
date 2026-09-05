declare namespace AdminService {
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
    id: string;
    create_date: string;
    email: string;
    is_active: '0' | '1';
    is_superuser: boolean;
    role: string;
    nickname: string;
    business_document_role:
      | 'AUTHOR_CREATOR'
      | 'AUTHOR_EDITOR'
      | 'MODERATOR_CREATOR'
      | 'EXTENDED_MODERATOR';
  };

  export type UserDetail = {
    avatar?: string;
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
    avatar?: string;
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
    avatar?: string;
    canvas_category: 'agent';
    permission: 'string';
    title: string;
  };

  export type TaskExecutorHeartbeatItem = {
    name: string;
    boot_at: string;
    now: string;
    ip_address: string;
    current: Record<string, object>;
    done: number;
    failed: number;
    lag: number;
    pending: number;
    pid: number;
  };

  export type TaskExecutorInfo = Record<string, TaskExecutorHeartbeatItem[]>;

  export type ListServicesItem = {
    extra: Record<string, unknown>;
    host: string;
    id: number;
    name: string;
    port: number;
    service_type: string;
    status: 'alive' | 'timeout' | 'fail';
  };

  export type ServiceDetail =
    | {
        service_name: string;
        status: 'alive' | 'timeout';
        message: string | Record<string, any> | Record<string, any>[];
      }
    | {
        service_name: 'task_executor';
        status: 'alive' | 'timeout';
        message: AdminService.TaskExecutorInfo;
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
    description: string;
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
    create_time: number;
    update_date: string;
    update_time: number;
  };

  // Sandbox settings types
  export type SandboxProvider = {
    id: string;
    name: string;
    description: string;
    tags: string[];
  };

  export type SandboxConfigFieldBase = {
    required?: boolean;
    label?: string;
    placeholder?: string;
    description?: string;
    multiline?: boolean;
    readonly?: boolean;
    scope?: 'runtime' | 'deployment';
  };

  export type SandboxConfigStringField = SandboxConfigFieldBase & {
    type: 'string';
    default?: string;
    secret?: boolean;
  };

  export type SandboxConfigIntegerField = SandboxConfigFieldBase & {
    type: 'integer';
    default?: number;
    min?: number;
    max?: number;
  };

  export type SandboxConfigBooleanField = SandboxConfigFieldBase & {
    type: 'boolean';
    default?: boolean;
  };

  export type SandboxConfigJsonField = SandboxConfigFieldBase & {
    type: 'json';
    default?: unknown;
  };

  export type SandboxConfigField =
    | SandboxConfigStringField
    | SandboxConfigIntegerField
    | SandboxConfigBooleanField
    | SandboxConfigJsonField;

  export type SandboxConfig = {
    provider_type: string;
    config: Record<string, unknown>;
  };

  export type NavigationVisibility = {
    visible_sections: import('@/constants/navigation').NavigationSection[];
  };

  export type AccessGroup = {
    id: string;
    name: string;
    description: string;
    user_ids: string[];
    dataset_ids: string[];
    sections: import('@/constants/navigation').NavigationSection[];
  };

  export type AccessGroupInput = Omit<AccessGroup, 'id'>;

  export type AccessGroupOptions = {
    users: Array<
      Pick<ListUsersItem, 'id' | 'email' | 'nickname' | 'is_superuser'>
    >;
    datasets: Array<{ id: string; name: string; tenant_id: string }>;
    sections: import('@/constants/navigation').NavigationSection[];
  };

  export type BusinessDocumentsEvaConnection = {
    api_base_url: string;
    web_base_url: string;
    project_id: string;
    verify_ssl: boolean;
    include_archived: boolean;
    token_configured: boolean;
  };

  export type BusinessDocumentsSettings = {
    eva_connection: BusinessDocumentsEvaConnection;
  };

  export type BusinessDocumentsEvaConnectionInput = Omit<
    BusinessDocumentsEvaConnection,
    'token_configured'
  > & {
    eva_api_token?: string;
    clear_token?: boolean;
  };

  export type AuditEventSource =
    | 'application'
    | 'business_documents'
    | 'ingestion'
    | 'connectors';

  export type AuditEventOutcome =
    | 'success'
    | 'failure'
    | 'pending'
    | 'cancelled';

  export type AuditEvent = {
    id: string;
    occurred_at: number;
    source: AuditEventSource;
    action: string;
    outcome: AuditEventOutcome;
    summary: string;
    actor: {
      id?: string;
      type: string;
      email?: string;
      nickname?: string;
    };
    object: {
      type: string;
      id: string;
      label: string;
    };
    correlation_id?: string | null;
    causation_id?: string | null;
    request_id?: string | null;
    trace_id?: string | null;
    span_id?: string | null;
    interaction_id?: string | null;
    job_id?: string | null;
    session_id?: string | null;
    error_id?: string | null;
    error?: {
      code?: string;
      message: string;
    } | null;
    details: Record<string, unknown>;
  };

  export type AuditEventQuery = {
    page?: number;
    page_size?: number;
    source?: AuditEventSource | '';
    outcome?: AuditEventOutcome | '';
    query?: string;
    actor?: string;
    correlation_id?: string;
  };

  export type AuditEventPage = {
    items: AuditEvent[];
    page: number;
    page_size: number;
    total: number;
    retention_days: number;
    unavailable_sources: AuditEventSource[];
    stats: {
      failures: number;
      sources: number;
    };
    observability: {
      enabled: boolean;
      grafana_url: string;
      loki_datasource_uid: string;
      tempo_datasource_uid: string;
    };
  };
}
