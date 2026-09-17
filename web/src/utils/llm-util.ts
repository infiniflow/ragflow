/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { IAddedModel } from '@/interfaces/database/llm';
import { getCachedLlmList } from './llm-cache';

// The names of the large models returned by the interface are similar to "deepseek-r1___OpenAI-API"
export function getRealModelName(llmName: string) {
  return llmName.split('__').at(0) ?? '';
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

/** Build "modelName@instanceName@providerName" */
export function buildModelValue(model: {
  model_name: string;
  model_instance: string;
  model_provider: string;
}) {
  return `${model.model_name}@${model.model_instance}@${model.model_provider}`;
}

/**
 * Collects every id under which an added model can be referenced — both the
 * model_id form and the legacy "modelName@instanceName@providerName" form —
 * so a persisted form value can be checked against the models that still
 * exist. Mirrors the leaf ids produced by `buildModelTree`.
 */
export function buildValidModelIds(
  allModels: IAddedModel[],
  modelTypes: string[],
): Set<string> {
  const ids = new Set<string>();
  for (const m of allModels) {
    if (!m.model_type?.some((t) => modelTypes.includes(t))) continue;
    const legacyId = buildModelValue({
      model_name: getRealModelName(m.name),
      model_instance: m.instance_name,
      model_provider: m.provider_name,
    });
    ids.add(m.model_id || legacyId);
    ids.add(legacyId);
  }
  return ids;
}

/**
 * Parse "modelName@instanceName@providerName" (or the 2-part
 * "modelName@providerName" form where the instance defaults to "default").
 *
 * The composite key is right-anchored: the *last* '@'-separated field is the
 * provider, the second-to-last is the instance, and everything to the left of
 * the second-to-last '@' is the bare model name. Some model names legitimately
 * contain '@' themselves (e.g. LM Studio embedding IDs such as
 * `text-embedding-nomic-embed-text-v1.5@q8_0`), producing four-`@` composite
 * keys like `text-embedding-nomic-embed-text-v1.5@q8_0@lmstudio@LM-Studio`.
 *
 * A naive `split("@")` (or anchoring on the first '@') mis-parses these keys
 * — PATCH /api/v1/models/default then sends `model_name="…v1.5"` and
 * `model_instance="q8_0@lmstudio"`, and the server replies
 * `Instance 'q8_0@lmstudio' not found for provider 'LM-Studio'`.
 *
 * Right-anchored split mirrors `api/db/joint_services/tenant_model_service.py`
 * `split_model_name` and the Go `parseModelName` (PR #16468 family).
 */
export function parseModelValue(val: string) {
  if (!val) return null;
  const lastAt = val.lastIndexOf('@');
  if (lastAt === -1) return null;
  const secondLastAt = val.lastIndexOf('@', lastAt - 1);
  if (secondLastAt === -1) {
    // 2-part form: "modelName@providerName" — instance defaults to "default".
    return {
      model_name: val.substring(0, lastAt),
      model_instance: 'default',
      model_provider: val.substring(lastAt + 1),
    };
  }
  return {
    model_name: val.substring(0, secondLastAt),
    model_instance: val.substring(secondLastAt + 1, lastAt),
    model_provider: val.substring(lastAt + 1),
  };
}

/**
 * Base embedding model name used to decide whether two datasets can be
 * selected and searched together. Composite references
 * ("modelName@instanceName@providerName" or "modelName@providerName") reduce
 * to the bare model name, so datasets using the same embedding model through
 * different provider instances still group together. Opaque values without
 * '@' (e.g. an unresolved tenant_model id) are returned unchanged, so only
 * exact matches group together. Mirrors the backend's base-name comparison
 * (Python `_base_model_name`, Go `common.BaseModelName`).
 */
export function getEmbeddingBaseName(embeddingModel?: string | null): string {
  if (!embeddingModel) return '';
  return parseModelValue(embeddingModel)?.model_name ?? embeddingModel;
}

// Extract model name and factory ID from a model UUID
// Supports both "model_name@factory_id" and "model_name@factory_id#instance_name".
// Uses right-anchored split for the same reason as parseModelValue:
// model names may contain '@' themselves, so a naive split('@') drops the
// last portion of the model name into factoryId.
export function parseModelUuid(uuid: string): {
  modelName: string;
  factoryId: string;
} {
  const hashIndex = uuid.indexOf('#');
  const core = hashIndex === -1 ? uuid : uuid.slice(0, hashIndex);
  const lastAt = core.lastIndexOf('@');
  if (lastAt === -1) {
    return { modelName: core, factoryId: '' };
  }
  return {
    modelName: core.substring(0, lastAt),
    factoryId: core.substring(lastAt + 1),
  };
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
// Note: /api/v1/chats is intentionally absent — the chats API normalizes the
// model name/id pair server-side, so the frontend submits only llm_id /
// rerank_id and must not have a stale tenant_* id injected back in here.
const API_WHITELIST = [
  '/api/v1/users/me/models',
  '/v1/canvas/set',
  '/v1/canvas/setting',
  '/api/v1/searches/',
  '/api/v1/memories',
  '/api/v1/datasets',
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

  // Handle arrays
  if (Array.isArray(data)) {
    return data.map((item) => addTenantParams(item, url));
  }

  const newData = { ...data };
  const llmList = getCachedLlmList();

  // Clear the paired tenant ID when a model selection is explicitly cleared.
  for (const [paramName, tenantParamName] of Object.entries(modelParamMap)) {
    if (
      Object.hasOwn(newData, paramName) &&
      (newData[paramName] === '' || newData[paramName] == null)
    ) {
      newData[tenantParamName] = null;
    }
  }

  if (!llmList) return newData;

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
