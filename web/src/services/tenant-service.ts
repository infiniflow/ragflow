import { request } from 'umi';

export interface Tenant {
  id: string;
  name: string;
  description?: string;
  llm_id: string;
  embd_id: string;
  asr_id: string;
  img2txt_id: string;
  rerank_id: string;
  tts_id: string;
  parser_ids: string;
  credit: number;
  status: string;
  create_time: string;
  update_time: string;
  role?: string;
}

export interface CreateTenantRequest {
  name: string;
  description?: string;
  llm_id?: string;
  embd_id?: string;
  asr_id?: string;
  img2txt_id?: string;
  rerank_id?: string;
  tts_id?: string;
  parser_ids?: string;
  credit?: number;
}

export interface TenantListResponse {
  tenants: Tenant[];
  total: number;
}

export interface TenantUsersResponse {
  users: any[];
  total: number;
}

export interface TenantUsageResponse {
  document_count: number;
  knowledgebase_count: number;
  conversation_count: number;
  tenant_id: string;
}

export interface TenantConfigResponse {
  tenant_id: string;
  name: string;
  description: string;
  llm_id: string;
  embd_id: string;
  asr_id: string;
  img2txt_id: string;
  rerank_id: string;
  tts_id: string;
  parser_ids: string;
  credit: number;
}

// Tenant management APIs
export async function getTenantList(params?: {
  page?: number;
  size?: number;
  keywords?: string;
  orderby?: string;
  desc?: boolean;
}): Promise<TenantListResponse> {
  return request('/api/v1/tenant_management/list', {
    method: 'GET',
    params,
  });
}

export async function createTenant(data: CreateTenantRequest): Promise<Tenant> {
  return request('/api/v1/tenant_management/create', {
    method: 'POST',
    data,
  });
}

export async function getTenant(tenantId: string): Promise<Tenant> {
  return request(`/api/v1/tenant_management/${tenantId}`);
}

export async function updateTenant(tenantId: string, data: Partial<CreateTenantRequest>): Promise<Tenant> {
  return request(`/api/v1/tenant_management/${tenantId}`, {
    method: 'PUT',
    data,
  });
}

export async function deleteTenant(tenantId: string): Promise<boolean> {
  return request(`/api/v1/tenant_management/${tenantId}`, {
    method: 'DELETE',
  });
}

export async function getTenantUsers(
  tenantId: string,
  params?: {
    page?: number;
    size?: number;
    role?: string;
    status?: string;
    keywords?: string;
  }
): Promise<TenantUsersResponse> {
  return request(`/api/v1/tenant_management/${tenantId}/users`, {
    method: 'GET',
    params,
  });
}

export async function updateUserRole(
  tenantId: string,
  userId: string,
  role: string
): Promise<boolean> {
  return request(`/api/v1/tenant_management/${tenantId}/users/${userId}/role`, {
    method: 'PUT',
    data: { role },
  });
}

export async function switchTenant(tenantId: string): Promise<{
  tenant_id: string;
  tenant_name: string;
  user_role: string;
}> {
  return request(`/api/v1/tenant_management/${tenantId}/switch`, {
    method: 'POST',
  });
}

export async function getTenantConfig(tenantId: string): Promise<TenantConfigResponse> {
  return request(`/api/v1/tenant_management/${tenantId}/config`);
}

export async function getTenantUsage(tenantId: string): Promise<TenantUsageResponse> {
  return request(`/api/v1/tenant_management/${tenantId}/usage`);
}