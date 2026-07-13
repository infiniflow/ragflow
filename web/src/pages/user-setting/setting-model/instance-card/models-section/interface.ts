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
   * (`api_key`, `base_url` / `api_base`, region-specific fields, ...).
   * When provided, ModelsSection prefers these over the persisted
   * `instance` props when calling listProviderModels / verifyProviderConnection,
   * so the user can verify with values they are still editing (before
   * blur-save persists them to the backend).
   */
  getFormValues?: () => Record<string, any>;
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
   * Optional callback fired when the per-instance model list changes
   * in a way that does NOT need the host to re-sync via its own
   * auto-save — i.e. an existing model was patched (max_tokens /
   * model_type / features changed via the edit dialog) but the model
   * set stayed the same.
   *
   * The PATCH endpoint already persisted the change server-side, so
   * the host uses this signal to absorb the resulting model_info diff
   * into its last-saved baseline. The next blur-driven auto-save will
   * then short-circuit as a no-op (signature matches), avoiding a
   * redundant PUT that re-sends the already-saved model_info.
   *
   * Pair with `onInstanceModelsChange`: that callback fires for every
   * change (add/remove/patch) so the host can keep its `modelInfoRef`
   * current, while `onInstanceModelsEdited` fires ONLY for patches so
   * the host can suppress its next auto-save for an already-persisted
   * change.
   */
  onInstanceModelsEdited?: () => void;
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
