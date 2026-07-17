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

import { LLMFactory } from '@/constants/llm';
import {
  useAddInstanceModel,
  useDeleteInstanceModels,
  useListProviderModels,
  usePatchInstanceModel,
  useUpdateProviderInstance,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { IInstanceModel, IProviderInstance } from '@/interfaces/database/llm';
import { IModelInfo, IProviderModelItem } from '@/interfaces/request/llm';
import {
  Dispatch,
  SetStateAction,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { sortModelTypes } from '../available-models';
import { useCustomModelFields } from '../use-custom-model-fields';
import { ModelsSectionProps, VerifyStatus } from './interface';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

/** Feature keys that mark a model as supporting tool/function calls. */
const TOOL_FEATURE_KEYS = ['is_tools', 'tool_call', 'tools', 'function_call'];

/** Sentinel instance name used by draft (unsaved) provider cards. */
export const DRAFT_INSTANCE_SENTINEL = '__draft__';

// ---------------------------------------------------------------------------
// Pure helpers (no React state, easy to test)
// ---------------------------------------------------------------------------

/** True when `features` contains any of {@link TOOL_FEATURE_KEYS}. */
export const hasToolFeature = (
  features: string[] | null | undefined,
): boolean =>
  Array.isArray(features) &&
  features.some((f) => TOOL_FEATURE_KEYS.includes(f));

/**
 * Normalize the assorted shapes returned by the backend for a model's
 * `model_types` into a plain `string[]`.
 *  - already an array → as-is
 *  - a single string   → wrapped
 *  - nullish / other   → []
 */
export const normalizeModelTypes = (raw: unknown): string[] =>
  Array.isArray(raw) ? raw : raw ? [raw as string] : [];

/**
 * Build an `IModelInfo[]` (the shape the PUT
 * `/providers/{name}/instances/{name}` endpoint expects) from a list of
 * provider model items. `features` is forwarded via `extra` so the backend
 * can persist per-model flags such as `is_tools`.
 */
export const buildModelInfo = (items: IProviderModelItem[]): IModelInfo[] =>
  items.map((m) => ({
    model_name: m.name,
    model_type: m.model_types ?? [],
    max_tokens: m.max_tokens ?? 0,
    extra: { is_tools: hasToolFeature(m.features) },
  }));

/** Resolved credentials for catalog / verify / batch calls. */
export type ResolvedCreds = { apiKey: string; baseUrl: string };

// ---------------------------------------------------------------------------
// 1. useResolveCreds — resolve api_key / base_url from host form or instance
// ---------------------------------------------------------------------------

export function useResolveCreds(
  instance: IProviderInstance | undefined,
  getFormValues: ModelsSectionProps['getFormValues'],
) {
  // Prefer the live values from the host card's form (so the user can
  // verify with an api_key they have just typed but not yet saved); fall
  // back to the persisted instance fields when no form getter is wired up.
  const resolveCreds = useCallback((): ResolvedCreds => {
    const fv = getFormValues?.() ?? {};
    return {
      apiKey: (fv.api_key as string) ?? instance?.api_key ?? '',
      baseUrl:
        (fv.base_url as string) ??
        (fv.api_base as string) ??
        instance?.base_url ??
        '',
    };
  }, [getFormValues, instance]);

  return { resolveCreds };
}

// ---------------------------------------------------------------------------
// 2. useModelsCatalog — upstream provider catalog fetch + auto-fetch
// ---------------------------------------------------------------------------

interface UseModelsCatalogArgs {
  providerName: string;
  instanceName: string;
  hideActions: boolean;
  resolveCreds: () => ResolvedCreds;
  instanceModels: IInstanceModel[] | undefined;
  /**
   * Current api_key value (read from the host form / instance). Used to
   * gate the auto-fetch for providers that require an api_key to list
   * models (currently only VolcEngine). For other providers the value
   * is ignored and the catalog is fetched on mount regardless.
   */
  apiKeyValue: string;
}

export function useModelsCatalog({
  providerName,
  instanceName,
  hideActions,
  resolveCreds,
  instanceModels,
  apiKeyValue,
}: UseModelsCatalogArgs) {
  const { listProviderModels } = useListProviderModels();
  const [catalog, setCatalog] = useState<IProviderModelItem[]>([]);
  const [manualListLoading, setManualListLoading] = useState(false);
  const [hasFetched, setHasFetched] = useState(false);

  // Manual "List models" handler — hits the upstream catalog endpoint.
  // The result is merged into `catalog`; the displayed list then becomes
  // the union of catalog + instance models.
  const handleListModels = async () => {
    const { apiKey, baseUrl } = resolveCreds();
    if (providerName === LLMFactory.VolcEngine && !apiKey) {
      setHasFetched(true);
      return;
    }
    setManualListLoading(true);
    try {
      const ret = await listProviderModels({
        provider_name: providerName,
        api_key: apiKey,
        base_url: baseUrl,
      });
      if (ret?.code === 0) {
        setCatalog((ret.data as IProviderModelItem[]) ?? []);
      }
      setHasFetched(true);
    } catch {
      setHasFetched(true);
    } finally {
      setManualListLoading(false);
    }
  };

  // Auto-fetch the provider's available-models catalog when this section
  // mounts (effectively "when the card is expanded"). For VolcEngine we
  // wait until an api_key is available (typed in the draft form or loaded
  // from instance details); for every other provider we fetch on mount
  // regardless of draft / saved state - the catalog endpoint does not
  // require credentials and the user expects to see the model list as
  // soon as they open the "Add instance" page.

  const requiresApiKey = providerName === LLMFactory.VolcEngine;
  const credsReady = !requiresApiKey || !!apiKeyValue;

  const hasAutoFetchedRef = useRef(false);
  useEffect(() => {
    if (hasAutoFetchedRef.current) return;
    if (hideActions) return;
    if (!providerName) return;
    if (!credsReady) return;
    hasAutoFetchedRef.current = true;
    handleListModels();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [providerName, instanceName, hideActions, credsReady]);

  // Mark `hasFetched` true once the per-instance query resolves — even if
  // it returned an empty array — so `hideIfEmpty` can safely take effect.
  useEffect(() => {
    if (instanceModels) {
      setHasFetched(true);
    }
  }, [instanceModels]);

  return {
    catalog,
    setCatalog,
    manualListLoading,
    hasFetched,
    handleListModels,
  };
}

// ---------------------------------------------------------------------------
// 3. useModelsDerived — derived model list (instance ∪ catalog) + sync
// ---------------------------------------------------------------------------

interface UseModelsDerivedArgs {
  catalog: IProviderModelItem[];
  instanceModels: IInstanceModel[] | undefined;
  /**
   * Locally-added models for a draft (unsaved) instance. The hook uses
   * this list as the "instance models" source when `isDraftInstance` is
   * true, so per-model add/remove/batch on a draft updates the derived
   * list without a backend round-trip. The host's save handler then
   * flushes the latest snapshot through `model_info` on save.
   */
  draftModels: IProviderModelItem[];
  /**
   * True when this card represents a draft instance (no backend id yet).
   * Picks between `instanceModels` (saved) and `draftModels` (draft) as
   * the source for `instanceItems` / `addedSet`.
   */
  isDraftInstance: boolean;
  onInstanceModelsChange: ModelsSectionProps['onInstanceModelsChange'];
  onInstanceModelsEdited?: ModelsSectionProps['onInstanceModelsEdited'];
}

export function useModelsDerived({
  catalog,
  instanceModels,
  draftModels,
  isDraftInstance,
  onInstanceModelsChange,
  onInstanceModelsEdited,
}: UseModelsDerivedArgs) {
  const catalogFeatures = useMemo(() => {
    const map = new Map<string, string[]>();
    catalog.forEach((m) => {
      if (Array.isArray(m.features) && m.features.length > 0) {
        map.set(m.name, m.features);
      }
    });
    return map;
  }, [catalog]);

  // For drafts the backend has no per-instance models yet, so the local
  // `draftModels` array stands in. For saved cards the backend list is
  // authoritative. The hook signature normalises both into the same
  // shape (`IProviderModelItem[]`) downstream.
  const sourceItems = useMemo(
    () => (isDraftInstance ? draftModels : ((instanceModels ?? []) as any[])),
    [isDraftInstance, draftModels, instanceModels],
  );

  const instanceItems: IProviderModelItem[] = useMemo(() => {
    // `im` is typed `any` because the backend may return either
    // `model_type` or `model_types`, and `features` is not on the
    // declared IInstanceModel interface.
    return sourceItems.map((im: any) => {
      const model_types = normalizeModelTypes(
        im.model_types ?? im.model_type ?? [],
      );
      const catalogFeats = catalogFeatures.get(im.name) ?? im.features ?? null;
      const features =
        im.is_tools && !hasToolFeature(catalogFeats)
          ? [...(catalogFeats ?? []), 'is_tools']
          : catalogFeats;
      return {
        name: im.name,
        max_tokens: im.max_tokens ?? 0,
        model_types,
        features,
      };
    });
  }, [sourceItems, catalogFeatures]);

  // Union of instance models + catalog, keyed by `name`. Catalog entries
  // win on conflict; instance set listed first so already-added models
  // stay at the top on the initial render.
  const models: IProviderModelItem[] = useMemo(() => {
    const byName = new Map<string, IProviderModelItem>();
    instanceItems.forEach((m) => byName.set(m.name, m));
    catalog.forEach((m) => byName.set(m.name, m));
    return Array.from(byName.values());
  }, [instanceItems, catalog]);

  // Mirror of `instanceItems` names - drives the +/- toggle on each row
  // and the batch-toggle button. For drafts this is the local "added"
  // set; for saved cards it tracks what the backend has persisted.
  const addedSet = useMemo(
    () => new Set(sourceItems.map((m: any) => m.name)),
    [sourceItems],
  );

  // Keep the latest callbacks in refs so the effect below only fires
  // when `instanceItems` actually changes — not on every parent
  // re-render that passes a new arrow for the callbacks. The previous
  // deps included the callbacks directly, which made the effect re-run
  // with the same data on every render; that was harmless for the
  // idempotent model_info push, but the new "edited" callback updates
  // the host's last-saved baseline and must not absorb in-flight form
  // edits fired by an unrelated re-render.
  const onChangeRef = useRef(onInstanceModelsChange);
  const onEditedRef = useRef(onInstanceModelsEdited);
  useEffect(() => {
    onChangeRef.current = onInstanceModelsChange;
    onEditedRef.current = onInstanceModelsEdited;
  });

  // Track the previous set of instance model names so we can tell
  // "patch" (same names, different data) apart from "add/remove"
  // (different names). Only the patch case needs to fire the host-side
  // baseline-update callback so the next blur auto-save short-circuits.
  const prevNamesRef = useRef<Set<string>>(new Set());

  // Push the latest per-instance model list up to the host so its
  // auto-save can include `model_info` in the payload. When the change
  // is purely a patch (same names, different data), also notify the
  // host via `onInstanceModelsEdited` so it can absorb the model_info
  // diff into its last-saved baseline — without this signal, the next
  // blur would signature-mismatch and fire a redundant PUT carrying
  // the already-PATCH-saved model_info. Adds/removes intentionally
  // skip this signal so the next blur still carries the updated list
  // into PUT (the standard sync path for the instance's model_info).
  useEffect(() => {
    const currentNames = new Set(instanceItems.map((m) => m.name));
    const prevNames = prevNamesRef.current;
    const isPatch =
      currentNames.size > 0 &&
      currentNames.size === prevNames.size &&
      Array.from(currentNames).every((n) => prevNames.has(n));

    onChangeRef.current?.(buildModelInfo(instanceItems));
    if (isPatch) {
      onEditedRef.current?.();
    }

    prevNamesRef.current = currentNames;
  }, [instanceItems]);

  return { instanceItems, models, addedSet };
}

// ---------------------------------------------------------------------------
// 4. useModelsFilter — search box + tag filter
// ---------------------------------------------------------------------------

export function useModelsFilter(models: IProviderModelItem[]) {
  const [search, setSearch] = useState('');
  const [tag, setTag] = useState<string | null>(null);

  const filteredModels = useMemo(() => {
    const q = search.trim().toLowerCase();
    return models.filter((m) => {
      if (q && !m.name.toLowerCase().includes(q)) return false;
      if (tag && !m.model_types?.includes(tag)) return false;
      return true;
    });
  }, [models, search, tag]);

  const allTags = useMemo(() => {
    const tagsSet = new Set<string>();
    models.forEach((m) => m.model_types?.forEach((t) => tagsSet.add(t)));
    return sortModelTypes(Array.from(tagsSet));
  }, [models]);

  return { search, tag, setSearch, setTag, filteredModels, allTags };
}

// ---------------------------------------------------------------------------
// 5. useModelVerify — per-model verify state + handler
// ---------------------------------------------------------------------------

interface UseModelVerifyArgs {
  providerName: string;
  resolveCreds: () => ResolvedCreds;
  instanceModels: IInstanceModel[] | undefined;
}

export function useModelVerify({
  providerName,
  resolveCreds,
  instanceModels,
}: UseModelVerifyArgs) {
  const { verifyProviderConnection } = useVerifyProviderConnection();
  const [verify, setVerify] = useState<Record<string, VerifyStatus>>({});

  // Seed the per-model verify status from the backend's persisted `verify`
  // flag on each instance model.
  useEffect(() => {
    if (!instanceModels || instanceModels.length === 0) return;
    setVerify((prev) => {
      let changed = false;
      const next = { ...prev };
      for (const im of instanceModels) {
        if (im.name in next) continue;
        if (im.verify === 'success') {
          next[im.name] = 'success';
          changed = true;
        } else if (im.verify === 'fail') {
          next[im.name] = 'error';
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [instanceModels]);

  const handleVerify = async (model: IProviderModelItem) => {
    setVerify((prev) => ({ ...prev, [model.name]: 'loading' }));
    try {
      const { apiKey, baseUrl } = resolveCreds();
      const ret = await verifyProviderConnection({
        provider_name: providerName,
        api_key: apiKey,
        base_url: baseUrl,
        model_info: [
          {
            model_name: model.name,
            model_type: model.model_types ?? [],
            max_tokens: model.max_tokens ?? 0,
          },
        ],
      });
      setVerify((prev) => ({
        ...prev,
        [model.name]: ret.code === 0 ? 'success' : 'error',
      }));
    } catch {
      setVerify((prev) => ({ ...prev, [model.name]: 'error' }));
    }
  };

  return { verify, handleVerify };
}

// ---------------------------------------------------------------------------
// 6. useModelMutations — add / remove / batch toggle / custom add
// ---------------------------------------------------------------------------

interface UseModelMutationsArgs {
  providerName: string;
  instanceName: string;
  isDraftInstance: boolean;
  hideActions: boolean;
  resolveCreds: () => ResolvedCreds;
  instance: IProviderInstance | undefined;
  instanceItems: IProviderModelItem[];
  filteredModels: IProviderModelItem[];
  addedSet: Set<string>;
  setCatalog: Dispatch<SetStateAction<IProviderModelItem[]>>;
  /**
   * Local mutators for the draft instance's model list. Required when
   * `isDraftInstance` is true so per-model add / remove / batch updates
   * stay local until the host saves the instance. Ignored for saved
   * cards (the backend mutations below fire as before).
   */
  addDraftModel?: (model: IProviderModelItem) => void;
  removeDraftModel?: (name: string) => void;
  setDraftModelsList?: (models: IProviderModelItem[]) => void;
}

export function useModelMutations({
  providerName,
  instanceName,
  isDraftInstance,
  hideActions,
  resolveCreds,
  instance,
  instanceItems,
  filteredModels,
  addedSet,
  setCatalog,
  addDraftModel,
  removeDraftModel,
  setDraftModelsList,
}: UseModelMutationsArgs) {
  const { addInstanceModel } = useAddInstanceModel();
  const { deleteInstanceModels } = useDeleteInstanceModels();
  const { updateProviderInstance, loading: batchLoading } =
    useUpdateProviderInstance();

  // True when every model currently shown in the filtered list is already
  // attached to the instance — drives the +/- toggle on the batch button.
  const allFilteredAdded = useMemo(
    () =>
      filteredModels.length > 0 &&
      filteredModels.every((m) => addedSet.has(m.name)),
    [filteredModels, addedSet],
  );

  const handleAddModel = async (model: IProviderModelItem) => {
    // Drafts have no backend instance yet — defer the call so the model
    // rides along with the instance save (model_info in the add body).
    if (isDraftInstance) {
      addDraftModel?.(model);
      return;
    }
    await addInstanceModel({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: model.name,
      model_type: model.model_types ?? [],
      max_tokens: model.max_tokens ?? 0,
      extra: { is_tools: hasToolFeature(model.features) },
    });
  };

  const handleRemoveModel = async (model: IProviderModelItem) => {
    if (isDraftInstance) {
      removeDraftModel?.(model.name);
      return;
    }
    await deleteInstanceModels({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: [model.name],
    });
  };

  const handleAddCustom = async (item: IProviderModelItem) => {
    // Append the custom item to the local catalog so it shows up in the
    // unioned `models` list immediately. Server-side persistence happens
    // via `addInstanceModel` below (when there is a real instance).
    setCatalog((prev) =>
      prev.some((m) => m.name === item.name) ? prev : [...prev, item],
    );
    if (hideActions || isDraftInstance) {
      // For drafts the catalog entry alone is not enough — we also need
      // to mark the model as added so it flows into the save payload's
      // `model_info`. Without this, custom models added on a draft
      // would render as "available" but not as "added", and would be
      // dropped on save.
      if (isDraftInstance) {
        addDraftModel?.(item);
      }
      return;
    }
    await addInstanceModel({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: item.name,
      model_type: item.model_types ?? [],
      max_tokens: item.max_tokens ?? 0,
      extra: { is_tools: hasToolFeature(item.features) },
    });
  };

  // Batch attach/detach the currently visible (filtered) models.
  //  - Saved card: PUT `/providers/{name}/instances/{name}` to replace
  //    `model_info` wholesale.
  //  - Draft: just rewrite the local draft list. The host save handler
  //    flushes the latest snapshot through the add-instance payload.
  const handleBatchToggleModels = async () => {
    if (filteredModels.length === 0) return;

    const byName = new Map<string, IProviderModelItem>();
    instanceItems.forEach((m) => byName.set(m.name, m));

    let nextModels: IProviderModelItem[];
    if (allFilteredAdded) {
      const drop = new Set(filteredModels.map((m) => m.name));
      nextModels = Array.from(byName.values()).filter((m) => !drop.has(m.name));
    } else {
      filteredModels.forEach((m) => byName.set(m.name, m));
      nextModels = Array.from(byName.values());
    }

    if (isDraftInstance) {
      setDraftModelsList?.(nextModels);
      return;
    }

    const { apiKey, baseUrl } = resolveCreds();
    await updateProviderInstance({
      provider_name: providerName,
      instance_name: instanceName,
      api_key: apiKey,
      base_url: baseUrl,
      region: instance?.region ?? 'default',
      model_info: buildModelInfo(nextModels),
    });
  };

  return {
    allFilteredAdded,
    handleAddModel,
    handleRemoveModel,
    handleAddCustom,
    handleBatchToggleModels,
    batchLoading,
  };
}

// ---------------------------------------------------------------------------
// 7. useModelEdit — edit dialog state + submit
// ---------------------------------------------------------------------------

interface UseModelEditArgs {
  providerName: string;
  instanceName: string;
  setCatalog: Dispatch<SetStateAction<IProviderModelItem[]>>;
}

export function useModelEdit({
  providerName,
  instanceName,
  setCatalog,
}: UseModelEditArgs) {
  const customModelDialogFields = useCustomModelFields();
  const { patchInstanceModel, loading: editLoading } = usePatchInstanceModel();
  // Model currently being edited via AddCustomModelDialog (with `name`
  // pinned/disabled and the dialog initial values pre-populated from the
  // model's current config). `null` when the edit dialog is closed.
  const [editingModel, setEditingModel] = useState<IProviderModelItem | null>(
    null,
  );

  // Field schema for the edit dialog — identical to the add schema
  // except the `name` field is locked (model name is the row's primary
  // key and the API forbids renaming via this endpoint).
  const editModelDialogFields = useMemo(
    () =>
      customModelDialogFields.map((f) =>
        f.name === 'name' ? { ...f, disabled: true } : f,
      ),
    [customModelDialogFields],
  );

  // Initial form values for the edit dialog, derived from the model
  // currently being edited.
  const editDefaultValues = useMemo(() => {
    if (!editingModel) return undefined;
    return {
      name: editingModel.name,
      model_types: editingModel.model_types ?? [],
      max_tokens: editingModel.max_tokens ?? 0,
      features: editingModel.features ?? [],
    };
  }, [editingModel]);

  // Persist edits to an existing model. The local `catalog` is patched so
  // the UI reflects the new values immediately, before the cache
  // invalidation lands.
  const handleEditSubmit = async (item: IProviderModelItem) => {
    if (!editingModel) return;
    const targetName = editingModel.name;

    setCatalog((prev) =>
      prev.some((m) => m.name === targetName)
        ? prev.map((m) => (m.name === targetName ? item : m))
        : prev,
    );

    await patchInstanceModel({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: targetName,
      max_tokens: item.max_tokens ?? 0,
      model_type: item.model_types ?? [],
      extra: { is_tools: hasToolFeature(item.features) },
    });
    setEditingModel(null);
  };

  return {
    editingModel,
    setEditingModel,
    editModelDialogFields,
    editDefaultValues,
    handleEditSubmit,
    editLoading,
    customModelDialogFields,
  };
}
