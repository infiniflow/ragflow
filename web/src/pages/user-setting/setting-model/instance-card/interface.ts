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

/**
 * Imperative save API exposed by each instance card (generic, Bedrock,
 * SoMark) so the parent page can drive a single batch save from the
 * top-of-page Save button.
 *
 * The card owns its form state and dirty tracking; the parent only needs
 * to validate, collect payloads, and dispatch the API calls.
 */
export interface ProviderInstanceCardRef {
  /**
   * Trigger form validation (and the draft-name check for drafts).
   * Returns true if the card is valid and ready to save. Errors are
   * surfaced in the form UI as a side effect of `trigger()`.
   */
  validate: () => Promise<boolean>;
  /**
   * Build the save payload for this card, or return `null` when there
   * is nothing to persist.
   *
   * - Drafts: always returns a payload (provided the instance name is
   *   non-empty), since a draft is by definition unsaved.
   * - Saved cards: returns a payload only when the current form values
   *   differ from the last-synced baseline; otherwise `null` so the
   *   parent can skip the redundant API call.
   */
  getSavePayload: () => InstanceSavePayload | null;
  /**
   * Update the card's dirty-tracking baseline to the current form
   * values. Called by the parent after a successful save so the next
   * `getSavePayload()` call short-circuits as a no-op.
   */
  markSaved: () => void;
}

/** Payload returned by {@link ProviderInstanceCardRef.getSavePayload}. */
export interface InstanceSavePayload {
  /** Ready-to-send body for the save API. Shape depends on `apiKind`. */
  payload: Record<string, any>;
  /** Instance name to save under. For drafts this is the typed-in name. */
  instanceName: string;
  /** True for a new (unsaved) instance, false for an existing one. */
  isDraft: boolean;
  /**
   * Which save endpoint the parent should dispatch to:
   *  - `'add'`: call `addProviderInstance` (drafts of any provider, plus
   *    Bedrock / SoMark saved cards which carry an `id` inside the
   *    `addProviderInstance` body).
   *  - `'update'`: call `updateProviderInstance` (generic saved cards,
   *    whose payload matches `IUpdateProviderInstanceRequestBody`).
   */
  apiKind: 'add' | 'update';
}

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
   * Renders the instance-name input section and treats all fields as
   * editable. Saving is driven by the parent through the imperative
   * ref API (see {@link ProviderInstanceCardRef}).
   */
  isDraft?: boolean;
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
  handleVerify: (params: any) => Promise<{ isValid: boolean; logs: string }>;
  handleDelete: () => Promise<void>;
  handleInstanceModelsEdited: () => void;
  providerName: string;
  /** Persisted instance name (from the backend). */
  instanceName: string;
  /**
   * The instance name currently displayed (may differ from `instanceName`
   * when the user has renamed via double-click). Falls back to
   * `instanceName` when not set.
   */
  editedInstanceName: string;
  /** Commit a rename: updates the card's edited-name state. */
  onRename: (name: string) => void;
  instance: IProviderInstance;
  instanceDetailsLoaded: boolean;
  modelInfoRef: React.MutableRefObject<IModelInfo[]>;
  draftName: string;
  open: boolean;
  setOpen: (open: boolean) => void;
}

/** Props for the draft-mode card (instance name + form fields). */
export interface DraftModeCardProps {
  formFields: FormFieldConfig[];
  formDefaultValues: Record<string, any>;
  formRef: RefObject<DynamicFormRef>;
  handleVerify: (params: any) => Promise<{ isValid: boolean; logs: string }>;
  handleDelete: () => Promise<void>;
  handleInstanceModelsEdited: () => void;
  providerName: string;
  instanceName: string;
  instance: IProviderInstance;
  modelInfoRef: React.MutableRefObject<IModelInfo[]>;
  draftName: string;
  setDraftName: (name: string) => void;
}
