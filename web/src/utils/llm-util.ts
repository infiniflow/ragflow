import { IThirdOAIModel } from '@/interfaces/database/llm';
import { getCachedLlmList } from './llm-cache';

export const getLLMIconName = (fid: string, llm_name: string) => {
  if (fid === 'FastEmbed') {
    return llm_name.split('/').at(0) ?? '';
  }

  return fid;
};

export const getLlmNameAndFIdByLlmId = (llmId?: string) => {
  const [llmName, fId] = llmId?.split('@') || [];

  return { fId, llmName };
};

// The names of the large models returned by the interface are similar to "deepseek-r1___OpenAI-API"
export function getRealModelName(llmName: string) {
  return llmName.split('__').at(0) ?? '';
}

export function buildLlmUuid(llm: IThirdOAIModel) {
  return `${llm.llm_name}@${llm.fid}`;
}

// Get tenant model ID from LLM list by model name and factory ID
export function getTenantModelId(
  llmList: Record<string, any>,
  modelName: string,
  factoryId: string,
): string {
  // Iterate through all providers in the LLM list
  for (const [provider, data] of Object.entries(llmList)) {
    if (data.llm && Array.isArray(data.llm)) {
      // Handle /v1/llm/my_llms format
      const model = data.llm.find(
        (m: any) => m.name === modelName && provider === factoryId,
      );
      if (model && model.id) {
        return model.id;
      }
    } else if (Array.isArray(data)) {
      // Handle /v1/llm/list format
      const model = data.find(
        (m: any) => m.llm_name === modelName && m.fid === factoryId,
      );
      if (model && model.id) {
        return model.id;
      }
    }
  }
  return '';
}

// Extract model name and factory ID from a model UUID (e.g., "model_name@factory_id")
export function parseModelUuid(uuid: string): {
  modelName: string;
  factoryId: string;
} {
  const [modelName, factoryId] = uuid.split('@');
  return { modelName, factoryId };
}

// Model parameter to tenant parameter mapping
type ModelParamMap = {
  [key: string]: string;
};

const modelParamMap: ModelParamMap = {
  llm_id: 'tenant_llm_id',
  embd_id: 'tenant_embd_id',
  asr_id: 'tenant_asr_id',
  tts_id: 'tenant_tts_id',
  img2txt_id: 'tenant_img2txt_id',
  rerank_id: 'tenant_rerank_id',
};

// API endpoint whitelist - only these endpoints will have tenant parameters added
const API_WHITELIST = [
  '/v1/user/set_tenant_info',
  '/v1/dialog/set',
  '/v1/canvas/set',
  '/v1/canvas/setting',
  '/v1/search/update',
  '/api/v1/memories',
  '/v1/kb/create',
  '/v1/kb/update',
  '/v1/dataflow/set',
];

// Check if the URL is in the whitelist
export function isUrlInWhitelist(url: string): boolean {
  return API_WHITELIST.some((endpoint) => url.includes(endpoint));
}

// Add tenant model ID parameters to request data
export function addTenantParams(data: any, url?: string): any {
  if (!data || typeof data !== 'object') return data;

  // If URL is provided and not in whitelist, return original data
  if (url && !isUrlInWhitelist(url)) {
    return data;
  }

  const llmList = getCachedLlmList();
  if (!llmList) return data;

  // Handle arrays
  if (Array.isArray(data)) {
    return data.map((item) => addTenantParams(item, url));
  }

  const newData = { ...data };

  // Iterate through model parameters and add corresponding tenant parameters
  for (const [paramName, tenantParamName] of Object.entries(modelParamMap)) {
    if (newData[paramName]) {
      try {
        const { modelName, factoryId } = parseModelUuid(newData[paramName]);
        const tenantModelId = getTenantModelId(llmList, modelName, factoryId);
        if (tenantModelId) {
          newData[tenantParamName] = tenantModelId;
        }
      } catch (error) {
        console.error(`Error processing ${paramName}:`, error);
      }
    }
  }

  // Recursively process nested objects
  for (const [key, value] of Object.entries(newData)) {
    if (value && typeof value === 'object' && !modelParamMap[key]) {
      newData[key] = addTenantParams(value, url);
    }
  }

  return newData;
}
