import api from '@/utils/api';
import request from '@/utils/request';

export interface SkillSpace {
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

export interface CreateSpaceRequest {
  name: string;
  description?: string;
  embd_id?: string;
  rerank_id?: string;
}

export interface UpdateSpaceRequest {
  name?: string;
  description?: string;
  embd_id?: string;
  rerank_id?: string;
  top_k?: number;
}

export interface SkillSearchConfig {
  id: string;
  tenant_id: string;
  space_id: string;
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
  space_id?: string;
  embd_id: string;
  vector_similarity_weight: number;
  similarity_threshold: number;
  field_config: Record<string, any>;
  rerank_id?: string;
  top_k: number;
}

export interface SearchRequest {
  tenant_id?: string;
  space_id?: string;
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
  space_id?: string;
  skills: SkillInfo[];
  embd_id?: string;
}

class SkillSpaceService {
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

    const jsonData = response?.data ?? response;

    if (jsonData?.code !== 0) {
      throw new Error(jsonData?.message || 'Request failed');
    }

    return jsonData.data;
  }

  // ==================== Skill Space Management ====================

  // List all skill spaces
  async listSpaces(): Promise<{ spaces: SkillSpace[]; total: number }> {
    return await this.request<{ spaces: SkillSpace[]; total: number }>(
      'GET',
      api.skillSpaces,
    );
  }

  // Create a new skill space
  async createSpace(request: CreateSpaceRequest): Promise<SkillSpace> {
    return await this.request<SkillSpace>('POST', api.skillSpaces, request);
  }

  // Get a skill space by ID
  async getSpace(spaceId: string): Promise<SkillSpace> {
    return await this.request<SkillSpace>('GET', api.skillSpace(spaceId));
  }

  // Update a skill space
  async updateSpace(
    spaceId: string,
    request: UpdateSpaceRequest,
  ): Promise<SkillSpace> {
    return await this.request<SkillSpace>(
      'PUT',
      api.skillSpace(spaceId),
      request,
    );
  }

  // Delete a skill space
  async deleteSpace(spaceId: string): Promise<void> {
    await this.request<void>('DELETE', api.skillSpace(spaceId));
  }

  // Get space by folder ID
  async getSpaceByFolder(folderId: string): Promise<SkillSpace> {
    return await this.request<SkillSpace>('GET', api.skillSpaceByFolder, null, {
      folder_id: folderId,
    });
  }

  // ==================== Skill Search Config ====================

  // Get skill search config
  async getConfig(
    spaceId?: string,
    embdId?: string,
  ): Promise<SkillSearchConfig> {
    const params: Record<string, string> = {};
    if (spaceId) params.space_id = spaceId;
    if (embdId) params.embd_id = embdId;

    return await this.request<SkillSearchConfig>(
      'GET',
      api.skillConfig,
      null,
      params,
    );
  }

  // Update skill search config
  async updateConfig(request: UpdateConfigRequest): Promise<SkillSearchConfig> {
    return await this.request<SkillSearchConfig>(
      'POST',
      api.skillConfig,
      request,
    );
  }

  // ==================== Skill Search ====================

  // Search skills
  async search(request: SearchRequest): Promise<SearchResult> {
    return await this.request<SearchResult>('POST', api.skillSearch, request);
  }

  // ==================== Skill Indexing ====================

  // Index skills
  async indexSkills(
    request: IndexSkillsRequest,
  ): Promise<{ indexed_count: number }> {
    return await this.request<{ indexed_count: number }>(
      'POST',
      api.skillIndex,
      request,
    );
  }

  // Delete skill index
  async deleteSkillIndex(skillId: string, spaceId?: string): Promise<void> {
    const params: Record<string, string> = { skill_id: skillId };
    if (spaceId) params.space_id = spaceId;

    await this.request<void>('DELETE', api.skillIndex, null, params);
  }

  // Reindex all skills
  async reindex(request: IndexSkillsRequest): Promise<any> {
    return await this.request<any>('POST', api.skillReindex, request);
  }
}

export const skillSpaceService = new SkillSpaceService();
export default skillSpaceService;
