import {
  IAddInstanceModelRequestBody,
  IAddProviderInstanceRequestBody,
} from '@/interfaces/request/llm';

const MODEL_RESERVED_KEYS = new Set([
  'llm_name',
  'model_name',
  'model_type',
  'max_tokens',
]);

const INSTANCE_RESERVED_KEYS = new Set([
  'instance_name',
  'llm_factory',
  'provider_name',
  'api_base',
  'base_url',
  'region',
  'verify',
]);

export const MODEL_EXTRA_KEYS = new Set([
  'is_tools',
  'vision',
  'provider_order',
  'api_version',
]);

export const MODEL_FIELD_NAMES = new Set<string>([
  ...MODEL_RESERVED_KEYS,
  ...MODEL_EXTRA_KEYS,
]);

export const isModelField = (fieldName: string) =>
  MODEL_FIELD_NAMES.has(fieldName);

type FlatPayload = Record<string, any>;

export type SplitResult = {
  instancePayload: Omit<
    IAddProviderInstanceRequestBody,
    'llm_name' | 'model_type' | 'max_tokens'
  > & {
    base_url?: string;
    region?: string;
  };
  modelPayload: IAddInstanceModelRequestBody;
};

const collectApiKeyExtras = (payload: FlatPayload) => {
  const extras: Record<string, any> = {};
  let apiKeyValue: any = undefined;
  for (const [key, value] of Object.entries(payload)) {
    if (value === undefined) continue;
    if (key === 'api_key') {
      apiKeyValue = value;
      continue;
    }
    if (INSTANCE_RESERVED_KEYS.has(key)) continue;
    if (MODEL_RESERVED_KEYS.has(key)) continue;
    if (MODEL_EXTRA_KEYS.has(key)) continue;
    extras[key] = value;
  }
  if (apiKeyValue && typeof apiKeyValue === 'object') {
    return { ...apiKeyValue, ...extras };
  }
  if (Object.keys(extras).length === 0) {
    return apiKeyValue ?? '';
  }
  if (apiKeyValue !== undefined && apiKeyValue !== '') {
    return { api_key: apiKeyValue, ...extras };
  }
  return extras;
};

const collectModelExtras = (payload: FlatPayload) => {
  const extras: Record<string, any> = {};
  for (const key of MODEL_EXTRA_KEYS) {
    if (payload[key] !== undefined && payload[key] !== '') {
      extras[key] = payload[key];
    }
  }
  return extras;
};

export const splitProviderPayload = (payload: FlatPayload): SplitResult => {
  const instancePayload = {
    instance_name: payload.instance_name as string,
    llm_factory: payload.llm_factory as string,
    api_key: collectApiKeyExtras(payload),
    base_url: (payload.base_url ?? payload.api_base) as string | undefined,
    region: (payload.region as string | undefined) || 'default',
  };

  const modelExtra = collectModelExtras(payload);

  const modelPayload = {
    model_name: (payload.model_name ?? payload.llm_name) as string,
    model_type: payload.model_type,
    max_tokens: payload.max_tokens as number,
    ...(Object.keys(modelExtra).length > 0 ? { extra: modelExtra } : {}),
  };

  return {
    instancePayload: instancePayload as SplitResult['instancePayload'],
    modelPayload,
  };
};
