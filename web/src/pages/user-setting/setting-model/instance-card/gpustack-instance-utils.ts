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

/** What a model is used as. Vision is a chat model that also accepts images. */
export type GPUStackModelKind =
  | 'chat'
  | 'vision'
  | 'embedding'
  | 'rerank'
  | 'asr'
  | 'tts';

/** Display order of the model groups. */
export const GPUSTACK_MODEL_KINDS: GPUStackModelKind[] = [
  'chat',
  'vision',
  'embedding',
  'rerank',
  'asr',
  'tts',
];

/** Used when GPUStack does not report a context length (audio models). */
export const GPUSTACK_DEFAULT_MAX_TOKENS = 8192;

export interface GPUStackModel {
  name: string;
  kind: GPUStackModelKind;
  maxTokens: number;
}

export type GPUStackConnectionError = 'auth' | 'unreachable' | 'other';

export function kindFromModelTypes(types?: string[] | null): GPUStackModelKind {
  const list = types ?? [];
  if (list.includes('vision') || list.includes('image2text')) return 'vision';
  if (list.includes('embedding')) return 'embedding';
  if (list.includes('rerank')) return 'rerank';
  if (list.includes('asr') || list.includes('speech2text')) return 'asr';
  if (list.includes('tts')) return 'tts';
  return 'chat';
}

export function modelTypesFromKind(kind: GPUStackModelKind): string[] {
  return kind === 'vision' ? ['chat', 'vision'] : [kind];
}

export function toGPUStackModel(item: {
  name: string;
  model_types?: string[] | null;
  max_tokens?: number | null;
}): GPUStackModel {
  return {
    name: item.name,
    kind: kindFromModelTypes(item.model_types),
    maxTokens: item.max_tokens || GPUSTACK_DEFAULT_MAX_TOKENS,
  };
}

/** Group models by kind, keeping only the groups that have a model. */
export function groupModelsByKind<T extends { kind: GPUStackModelKind }>(
  models: T[],
): { kind: GPUStackModelKind; models: T[] }[] {
  return GPUSTACK_MODEL_KINDS.map((kind) => ({
    kind,
    models: models.filter((model) => model.kind === kind),
  })).filter((group) => group.models.length > 0);
}

/** RAGFlow runs in a container, where localhost is not the host machine. */
export function isLoopbackUrl(baseUrl: string): boolean {
  try {
    const { hostname } = new URL(baseUrl.trim());
    return ['localhost', '127.0.0.1', '0.0.0.0', '[::1]'].includes(hostname);
  } catch {
    return false;
  }
}

export function classifyConnectionError(
  message?: string,
): GPUStackConnectionError {
  const text = (message ?? '').toLowerCase();
  if (/status (401|403)\b/.test(text)) return 'auth';
  if (
    /failed to send request|connection refused|no such host|dial tcp|timeout|deadline exceeded|unreachable/.test(
      text,
    )
  ) {
    return 'unreachable';
  }
  return 'other';
}

/** Compare what GPUStack serves with what this connection already has. */
export function diffModels(
  discovered: GPUStackModel[],
  added: GPUStackModel[],
): {
  notAdded: GPUStackModel[];
  added: (GPUStackModel & { running: boolean })[];
} {
  const addedNames = new Set(added.map((model) => model.name));
  const servedNames = new Set(discovered.map((model) => model.name));
  return {
    notAdded: discovered.filter((model) => !addedNames.has(model.name)),
    added: added.map((model) => ({
      ...model,
      running: servedNames.has(model.name),
    })),
  };
}
