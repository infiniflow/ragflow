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

import { DynamicFormRef, FormFieldConfig } from '@/components/dynamic-form';
import { IProviderInstance } from '@/interfaces/database/llm';
import { IModelInfo } from '@/interfaces/request/llm';
import { RefObject } from 'react';

/** Public props for {@link ProviderInstanceCard}. */
export interface ProviderInstanceCardProps {
  providerName: string;
  /**
   * The instance to render. When `isDraft` is true, this is a placeholder
   * used to render the "new instance" inline form; the actual save call
   * will use the values typed in the form fields.
   */
  instance: IProviderInstance;
  /**
   * True when this card represents a freshly-added (unsaved) instance.
   * Renders Save / Cancel buttons and treats all fields as editable.
   */
  isDraft?: boolean;
  /** Called after a draft instance is successfully saved. */
  onSaved?: (values: Record<string, any>) => void | Promise<void>;
  /**
   * Called after a draft instance's *name* has been persisted via
   * `addProviderInstance` (with just `instance_name`). The parent should
   * remove this draft from its visible list; the freshly invalidated
   * `providerInstances` query will surface the persisted card. The
   * saved `instanceName` is passed so the parent can keep the newly
   * persisted card expanded.
   */
  onNameSaved?: (instanceName: string) => void;
  /**
   * Called when the user deletes a draft instance.
   * For drafts this is equivalent to onCancel; for saved instances
   * the component calls useDeleteProviderInstance internally.
   */
  onDelete?: () => void;
  /**
   * When true, this card starts expanded and its instance details
   * are fetched on mount. Default `false` so additional cards stay
   * collapsed until the user opens them — at which point details
   * are fetched on demand.
   */
  defaultOpen?: boolean;
}

/**
 * Provider-specific credential fields that the backend expects bundled
 * *inside* `api_key` as an object rather than as top-level keys:
 *   api_key: { api_key, group_id?, api_version?, provider_order? }
 * - MiniMax        → group_id
 * - Azure OpenAI   → api_version
 * - OpenRouter     → provider_order
 * When none of these are present the api_key stays a bare string.
 */
export const API_KEY_NESTED_FIELDS = [
  'group_id',
  'api_version',
  'provider_order',
] as const;

export type ApiKeyNestedField = (typeof API_KEY_NESTED_FIELDS)[number];

/** Sentinel instance name used by draft (unsaved) provider cards. */
export const DRAFT_INSTANCE_SENTINEL = '__draft__';

// ---------------------------------------------------------------------------
// Sub-component props
// ---------------------------------------------------------------------------

/** Props for the saved-mode (collapsible) card body. */
export interface SavedModeCardProps {
  formFields: FormFieldConfig[];
  formDefaultValues: Record<string, any>;
  formRef: RefObject<DynamicFormRef>;
  handleFieldsBlur: (e: React.FocusEvent<HTMLDivElement>) => void;
  handleVerify: (params: any) => Promise<{ isValid: boolean; logs: string }>;
  handleDelete: () => Promise<void>;
  handleInstanceModelsEdited: () => void;
  providerName: string;
  instanceName: string;
  instance: IProviderInstance;
  instanceDetailsLoaded: boolean;
  modelInfoRef: React.MutableRefObject<IModelInfo[]>;
  blurSuppressRef: React.MutableRefObject<boolean>;
  draftName: string;
  open: boolean;
  setOpen: (open: boolean) => void;
}

/** Props for the draft-mode card (instance name + locked form fields). */
export interface DraftModeCardProps {
  formFields: FormFieldConfig[];
  formDefaultValues: Record<string, any>;
  formRef: RefObject<DynamicFormRef>;
  handleVerify: (params: any) => Promise<{ isValid: boolean; logs: string }>;
  handleDelete: () => Promise<void>;
  handleSaveName: () => Promise<void>;
  handleInstanceModelsEdited: () => void;
  providerName: string;
  instanceName: string;
  instance: IProviderInstance;
  modelInfoRef: React.MutableRefObject<IModelInfo[]>;
  draftName: string;
  setDraftName: (name: string) => void;
}
