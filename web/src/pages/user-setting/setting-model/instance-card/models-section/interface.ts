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

import { IProviderInstance } from '@/interfaces/database/llm';
import { IModelInfo, IProviderModelItem } from '@/interfaces/request/llm';

/** State of the per-row "verify model" button. */
export type VerifyStatus = 'idle' | 'loading' | 'success' | 'error';

export interface ModelsSectionProps {
  providerName: string;
  instanceName: string;
  /** Optional — used to populate api_key/base_url for the verify and list calls. */
  instance?: IProviderInstance;
  /**
   * If true, hides the List Models / + buttons (used in the "new instance"
   * draft state where there is no backend instance to query yet).
   */
  hideActions?: boolean;
  /**
   * True once the lazy-loaded instance details (which carry `api_key` /
   * `base_url` - the list endpoint omits them) have resolved. Providers
   * whose upstream model-list endpoint requires an api_key (e.g.
   * VolcEngine) use this to defer the auto-fetch until the credential
   * is available in the host form, instead of firing a request that is
   * guaranteed to fail and then refusing to retry.
   */
  instanceDetailsLoaded?: boolean;
  /**
   * If true, the section renders nothing once the first catalog fetch
   * completes and no models are available. Used by draft instances to
   * avoid showing an empty list.
   */
  hideIfEmpty?: boolean;
  /**
   * Optional getter returning the host card's current form values
   * (`api_key`, `base_url`, region-specific fields, ...).
   * When provided, ModelsSection prefers these over the persisted
   * `instance` props when calling listProviderModels / verifyProviderConnection,
   * so the user can verify with values they are still editing (before
   * blur-save persists them to the backend).
   */
  getFormValues?: () => Record<string, any>;
  /**
   * Optional provider-specific transform used to build the verify
   * payload's `api_key` / `base_url` / `region` from the host card's
   * current form values. Providers whose credential field names don't
   * map directly onto `api_key` / `base_url` (e.g. PaddleOCR's nested
   * `paddleocr_api_url` / `paddleocr_access_token` / `paddleocr_algorithm`)
   * supply this so the per-model verify can forward the structured
   * `api_key` object the backend expects. When absent the generic
   * `values.api_key` / `values.base_url` mapping is used.
   *
   * `modelInfo` returned by the transform is ignored for per-model
   * verify - the single model being verified always overrides it.
   */
  verifyTransform?: (values: Record<string, any>) => {
    apiKey: string | object | Record<string, any>;
    baseUrl?: string;
    region?: string;
    modelInfo?: IModelInfo[];
  };
  /**
   * Notifies the host that ModelsSection has opened (or closed) a modal
   * dialog whose contents live in a React Portal outside the host's
   * `onBlurCapture` container. The host should temporarily disable its
   * blur-driven auto-save while suppressed === true; otherwise the
   * focus shift into the dialog body fires a spurious "save". Restored
   * to false when the dialog closes.
   */
  onBlurSuppressChange?: (suppressed: boolean) => void;
  /**
   * Notifies the host whenever the per-instance model list changes.
   * The list is delivered already converted to the `IModelInfo[]`
   * shape expected by the update / add-provider-instance endpoints,
   * so the host can forward it verbatim in its auto-save payload.
   * Fires once on mount with `[]` (initial empty state) and again
   * whenever the per-instance fetch resolves or an add/remove mutation
   * settles and the cache invalidates.
   */
  onInstanceModelsChange?: (modelInfo: IModelInfo[]) => void;
  /**
   * Optional callback fired after saved-instance model state is confirmed by
   * the backend, either from a query result or a successful edit. The host
   * uses it to absorb `model_info` into its last-saved baseline, avoiding a
   * redundant update for data that the backend has already persisted.
   */
  onInstanceModelsEdited?: () => void;
  /** Reports whether the saved instance model snapshot is authoritative. */
  onInstanceModelsStatusChange?: (ready: boolean) => void;
}

export interface ModelTypeBadgesProps {
  types: string[];
  showEdit?: boolean;
  onEdit?: () => void;
  editLabel?: string;
  editTestSuffix?: string;
}

export interface ModelVerifyButtonProps {
  status: VerifyStatus;
  onVerify: () => void;
  modelName: string;
}

export interface ModelRowProps {
  model: IProviderModelItem;
  isAdded: boolean;
  verifyStatus: VerifyStatus;
  hideActions: boolean;
  onVerify: () => void;
  onAdd: () => void;
  onRemove: () => void;
  onEdit: () => void;
  editLabel: string;
}

export interface TagFilterButtonProps {
  label: string;
  count: number;
  active: boolean;
  onClick: () => void;
}
