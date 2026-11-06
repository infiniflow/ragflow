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

import { IOntologyBinding } from '@/interfaces/database/chat';

/**
 * How this assistant relates to the knowledge layer.
 *
 * Inherit is not a weaker "on": it is the absence of an assistant-level setting,
 * which leaves whatever the deployment declared on the agentic template in
 * force. Off is the opposite — it blocks that declaration. Collapsing the three
 * into a switch would make "turn it off for this one assistant" impossible.
 */
export const OntologyOverrideMode = {
  Inherit: 'inherit',
  On: 'on',
  Off: 'off',
} as const;

export type OntologyOverrideModeValue =
  (typeof OntologyOverrideMode)[keyof typeof OntologyOverrideMode];

export const OntologyVersionFollowPublished = 0;

export type OntologyBindingFields = {
  modelId?: string;
  modelVersion?: number;
  knowledgeDatasetIds?: string[];
};

export type OntologyBindingIssue = {
  path: string[];
  message: string;
};

export type OntologyBindingValue = {
  prompt_config?: {
    ontology?: IOntologyBinding | null;
  };
};

/**
 * Reads the mode out of a stored binding.
 *
 * An absent `enabled` means on, not off: the server treats a payload as a
 * binding unless it is explicitly disabled, so a half-written object must not be
 * read as a request to turn the layer off.
 */
export function ontologyOverrideMode(
  binding?: IOntologyBinding | null,
): OntologyOverrideModeValue {
  if (!binding) {
    return OntologyOverrideMode.Inherit;
  }
  return binding.enabled === false
    ? OntologyOverrideMode.Off
    : OntologyOverrideMode.On;
}

/**
 * Builds the value to store for a mode.
 *
 * Inherit returns null rather than failing to write a key, because the server
 * merges a patch into the stored prompt_config key by key: a key left out keeps
 * its previous value, so an override could never be removed. null is the wire
 * form of "no assistant-level setting", which is what puts the template's
 * default back in force.
 */
export function ontologyBindingPayload(
  mode: OntologyOverrideModeValue,
  fields: OntologyBindingFields,
): IOntologyBinding | null {
  if (mode === OntologyOverrideMode.Inherit) {
    return null;
  }

  if (mode === OntologyOverrideMode.Off) {
    return { enabled: false };
  }

  return {
    enabled: true,
    model_id: (fields.modelId ?? '').trim(),
    model_version: fields.modelVersion ?? OntologyVersionFollowPublished,
    knowledge_dataset_ids: fields.knowledgeDatasetIds ?? [],
  };
}

/**
 * What stops a binding from taking effect.
 *
 * A binding is complete or it is nothing: a model without a knowledge scope has
 * nowhere to look, and a scope without a model has no declarations to read. The
 * server resolves such a pair to no binding at all, silently — which is why it
 * is refused here instead.
 */
export function ontologyBindingIssues(
  value: OntologyBindingValue,
  t: (key: string) => string,
): OntologyBindingIssue[] {
  const binding = value?.prompt_config?.ontology;
  if (!binding || ontologyOverrideMode(binding) === OntologyOverrideMode.Off) {
    return [];
  }

  const issues: OntologyBindingIssue[] = [];
  if (!binding.model_id?.trim()) {
    issues.push({
      path: ['prompt_config', 'ontology', 'model_id'],
      message: t('ontologyModelRequired'),
    });
  }
  if (!binding.knowledge_dataset_ids?.length) {
    issues.push({
      path: ['prompt_config', 'ontology', 'knowledge_dataset_ids'],
      message: t('ontologyKnowledgeScopeRequired'),
    });
  }
  return issues;
}
