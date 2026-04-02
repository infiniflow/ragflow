import request from '@/utils/request';

const API_PREFIX = '/api/v1';

export interface SkillsHub {
  id: string;
  tenant_id: string;
  name: string;
  folder_id: string;
  description?: string;
  embd_id?: string;
  rerank_id?: string;
  top_k?: number;
  status?: string;
  create_time?: number;
  update_time?: string;
}

export interface CreateHubRequest {
  name: string;
  description?: string;
  embd_id?: string;
  rerank_id?: string;
}

export interface UpdateHubRequest {
  name?: string;
  description?: string;
  embd_id?: string;
  rerank_id?: string;
  top_k?: number;
}

export interface SkillSearchConfig {
  id: string;
  tenant_id: string;
  hub_id: string;
  embd_id: string;
  vector_similarity_weight: number;
  similarity_threshold: number;
  field_config: Record<string, any>;
  rerank_id?: string;
  tenant_rerank_id?: number;
  top_k: number;
  index_version: string;
  status: string;
  create_time?: number;
  update_time?: string;
}

export interface UpdateConfigRequest {
  tenant_id?: string;
  hub_id?: string;
  embd_id: string;
  vector_similarity_weight: number;
  similarity_threshold: number;
  field_config: Record<string, any>;
  rerank_id?: string;
  top_k: number;
}

export interface SearchRequest {
  tenant_id?: string;
  hub_id?: string;
  query: string;
  page?: number;
  page_size?: number;
}

export interface SearchResult {
  skills: Array<{
    skill_id: string;
    folder_id: string;
    name: string;
    description: string;
    tags: string[];
    score: number;
    bm25_score?: number;
    vector_score?: number;
    index_version?: string;
  }>;
  total: number;
  query: string;
  search_type: string;
}

export interface SkillInfo {
  id: string;
  folder_id: string;
  name: string;
  description: string;
  tags: string[];
  content: string;
}

export interface IndexSkillsRequest {
  tenant_id?: string;
  hub_id?: string;
  skills: SkillInfo[];
  embd_id?: string;
}

class SkillsHubService {
  private async request<T>(
    method: string,
    url: string,
    data?: any,
    params?: any,
  ): Promise<T> {
    const response: any = await request(url, {
      method: method as any,
      data,
      params,
    });

    // Handle both direct response and response with data property
    const jsonData = response?.data ?? response;

    if (jsonData?.code !== 0) {
      throw new Error(jsonData?.message || 'Request failed');
    }

    return jsonData.data;
  }

  // ==================== Skills Hub Management ====================

  // List all skills hubs
  async listHubs(): Promise<{ hubs: SkillsHub[]; total: number }> {
    return await this.request<{ hubs: SkillsHub[]; total: number }>(
      'GET',
      `${API_PREFIX}/skills/hubs`,
    );
  }

  // Create a new skills hub
  async createHub(request: CreateHubRequest): Promise<SkillsHub> {
    return await this.request<SkillsHub>(
      'POST',
      `${API_PREFIX}/skills/hubs`,
      request,
    );
  }

  // Get a skills hub by ID
  async getHub(hubId: string): Promise<SkillsHub> {
    return await this.request<SkillsHub>(
      'GET',
      `${API_PREFIX}/skills/hubs/${hubId}`,
    );
  }

  // Update a skills hub
  async updateHub(
    hubId: string,
    request: UpdateHubRequest,
  ): Promise<SkillsHub> {
    return await this.request<SkillsHub>(
      'PUT',
      `${API_PREFIX}/skills/hubs/${hubId}`,
      request,
    );
  }

  // Delete a skills hub
  async deleteHub(hubId: string): Promise<void> {
    await this.request<void>('DELETE', `${API_PREFIX}/skills/hubs/${hubId}`);
  }

  // Get hub by folder ID
  async getHubByFolder(folderId: string): Promise<SkillsHub> {
    return await this.request<SkillsHub>(
      'GET',
      `${API_PREFIX}/skills/hub/by-folder`,
      null,
      { folder_id: folderId },
    );
  }

  // ==================== Skill Search Config ====================

  // Get skill search config
  async getConfig(hubId?: string, embdId?: string): Promise<SkillSearchConfig> {
    const params: Record<string, string> = {};
    if (hubId) params.hub_id = hubId;
    if (embdId) params.embd_id = embdId;

    return await this.request<SkillSearchConfig>(
      'GET',
      `${API_PREFIX}/skills/config`,
      null,
      params,
    );
  }

  // Update skill search config
  async updateConfig(request: UpdateConfigRequest): Promise<SkillSearchConfig> {
    return await this.request<SkillSearchConfig>(
      'POST',
      `${API_PREFIX}/skills/config`,
      request,
    );
  }

  // ==================== Skill Search ====================

  // Search skills
  async search(request: SearchRequest): Promise<SearchResult> {
    return await this.request<SearchResult>(
      'POST',
      `${API_PREFIX}/skills/search`,
      request,
    );
  }

  // ==================== Skill Indexing ====================

  // Index skills
  async indexSkills(
    request: IndexSkillsRequest,
  ): Promise<{ indexed_count: number }> {
    return await this.request<{ indexed_count: number }>(
      'POST',
      `${API_PREFIX}/skills/index`,
      request,
    );
  }

  // Delete skill index
  async deleteSkillIndex(skillId: string, hubId?: string): Promise<void> {
    const params: Record<string, string> = {};
    if (hubId) params.hub_id = hubId;

    await this.request<void>(
      'DELETE',
      `${API_PREFIX}/skills/index/${skillId}`,
      null,
      params,
    );
  }

  // Reindex all skills
  async reindex(request: IndexSkillsRequest): Promise<any> {
    return await this.request<any>(
      'POST',
      `${API_PREFIX}/skills/reindex`,
      request,
    );
  }
}

export const skillsHubService = new SkillsHubService();
export default skillsHubService;
