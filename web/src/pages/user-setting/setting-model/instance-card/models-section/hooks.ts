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
import { useQueryClient } from '@tanstack/react-query';
import {
  LlmKeys,
  useAddInstanceModel,
  useDeleteInstanceModels,
  useListProviderModels,
  usePatchInstanceModel,
  useUpdateProviderInstance,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { IInstanceModel, IProviderInstance } from '@/interfaces/database/llm';
import { IModelInfo, IProviderModelItem } from '@/interfaces/request/llm';
import llmService from '@/services/llm-service';
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
    extra: { is_tools: hasToolFeature(m.features), ...(m.extra ?? {}) },
  }));

/** Resolved credentials for catalog / verify / batch calls.
 *  `baseUrl` is `undefined` when the provider's form has no `base_url`
 *  field (e.g. VolcEngine, Google Cloud) so the auto-fetch gate can
 *  distinguish "no base_url field" from "base_url field exists but is
 *  empty". */
export type ResolvedCreds = {
  apiKey: string | object;
  baseUrl: string | undefined;
};

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
      apiKey: (fv.api_key as string | object) ?? instance?.api_key ?? '',
      baseUrl: (fv.base_url as string) ?? instance?.base_url,
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

  apiKeyValue: ResolvedCreds['apiKey'];

  baseUrlValue: string | undefined;

  instanceDetailsLoaded?: boolean;
}

export function useModelsCatalog({
  providerName,
  instanceName,
  hideActions,
  resolveCreds,
  instanceModels,
  apiKeyValue,
  baseUrlValue,
  instanceDetailsLoaded,
}: UseModelsCatalogArgs) {
  const { listProviderModels } = useListProviderModels();
  const [catalog, setCatalog] = useState<IProviderModelItem[]>([]);
  const [catalogOverrides, setCatalogOverrides] = useState<
    Record<string, IProviderModelItem>
  >({});
  const catalogOverridesRef = useRef(catalogOverrides);
  const [manualListLoading, setManualListLoading] = useState(false);
  const [hasFetched, setHasFetched] = useState(false);

  const applyCatalogOverrides = useCallback((items: IProviderModelItem[]) => {
    const overrides = catalogOverridesRef.current;
    const names = new Set<string>();
    const merged = items.map((item) => {
      names.add(item.name);
      const override = overrides[item.name];
      return override ? { ...item, ...override, name: item.name } : item;
    });
    Object.entries(overrides).forEach(([name, override]) => {
      if (!names.has(name)) {
        merged.push(override);
      }
    });
    return merged;
  }, []);

  const updateCatalogModel = useCallback(
    (name: string, item: IProviderModelItem) => {
      setCatalogOverrides((prev) => {
        const next = {
          ...prev,
          [name]: { ...(prev[name] ?? {}), ...item, name },
        };
        catalogOverridesRef.current = next;
        return next;
      });
      setCatalog((prev) => {
        if (!prev.some((m) => m.name === name)) {
          return [...prev, { ...item, name }];
        }
        return prev.map((m) => (m.name === name ? { ...m, ...item, name } : m));
      });
    },
    [],
  );

  const clearCatalogOverride = useCallback((name: string) => {
    setCatalogOverrides((prev) => {
      if (!prev[name]) return prev;
      const next = { ...prev };
      delete next[name];
      catalogOverridesRef.current = next;
      return next;
    });
  }, []);

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
        api_key: apiKey as any,
        base_url: baseUrl,
      });
      if (ret?.code === 0) {
        setCatalog(
          applyCatalogOverrides((ret.data as IProviderModelItem[]) ?? []),
        );
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
  // from instance details). For providers whose form includes a
  // `base_url` field (e.g. Ollama, Xinference, LocalAI) we defer until a
  // non-empty URL is entered - their list-models endpoint needs the URL
  // to know which server to query. For every other provider we fetch on
  // mount regardless.
  //
  // The credential check is performed INSIDE the effect (not in the deps).
  // The host restores saved credentials before passive effects run, so this
  // reads the current form snapshot instead of the previous render's values.

  const hasAutoFetchedRef = useRef(false);
  useEffect(() => {
    if (hasAutoFetchedRef.current) return;
    if (hideActions) return;
    if (!providerName) return;
    if (
      providerName === LLMFactory.Bedrock &&
      instanceName === DRAFT_INSTANCE_SENTINEL
    )
      return;

    const creds = resolveCreds();
    const apiKeyConfig =
      typeof creds.apiKey === 'object' && creds.apiKey !== null
        ? (creds.apiKey as Record<string, unknown>)
        : undefined;
    const requiresApiKey =
      providerName === LLMFactory.VolcEngine ||
      (providerName === LLMFactory.Bedrock &&
        apiKeyConfig?.auth_mode === 'bedrock_api_key');
    const hasApiKey =
      providerName === LLMFactory.Bedrock && requiresApiKey
        ? Boolean(apiKeyConfig?.bedrock_api_key && apiKeyConfig?.bedrock_region)
        : Boolean(creds.apiKey);
    const waitingForInstanceDetails =
      providerName === LLMFactory.Bedrock &&
      instanceName !== DRAFT_INSTANCE_SENTINEL &&
      !instanceDetailsLoaded;
    const hasBaseUrlField = creds.baseUrl !== undefined;
    const ready =
      !waitingForInstanceDetails &&
      (!requiresApiKey || hasApiKey) &&
      (!hasBaseUrlField || !!creds.baseUrl);
    if (!ready) return;
    hasAutoFetchedRef.current = true;
    handleListModels();
    // oxlint-disable-next-line react/exhaustive-deps
  }, [
    providerName,
    instanceName,
    hideActions,
    apiKeyValue,
    baseUrlValue,
    instanceDetailsLoaded,
  ]);

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
    updateCatalogModel,
    clearCatalogOverride,
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
  /** True while the saved instance model query is fetching. */
  instanceModelsLoading: boolean;
  /** True only when the saved instance model query completed successfully. */
  instanceModelsSucceeded: boolean;
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
  instanceModelsLoading,
  instanceModelsSucceeded,
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
        extra: im.extra,
      };
    });
  }, [sourceItems, catalogFeatures]);

  // Union of instance models + catalog, keyed by `name`. Instance entries
  // win on conflict so that editing an already-added model loads the
  // instance-specific values (e.g. a user-customised `max_tokens`) rather
  // than the upstream catalog defaults; catalog entries are only used to
  // fill in models that have not been added yet. Instance set is listed
  // first so already-added models stay at the top on the initial render.
  const models: IProviderModelItem[] = useMemo(() => {
    const byName = new Map<string, IProviderModelItem>();
    instanceItems.forEach((m) => byName.set(m.name, m));
    catalog.forEach((m) => {
      if (!byName.has(m.name)) {
        byName.set(m.name, m);
      }
    });
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

  // Push the latest per-instance model list up to the host so its
  // save payload can include `model_info`.
  useEffect(() => {
    onChangeRef.current?.(buildModelInfo(instanceItems));
  }, [instanceItems]);

  // Saved instance models come from the backend, both after the initial
  // query and after mutation-driven refetches. Once that authoritative
  // snapshot is ready, absorb it into the host's saved baseline so an
  // unchanged card does not look dirty solely because model_info loaded
  // after the instance details. Draft models remain local and must still
  // be included in the first save.
  useEffect(() => {
    if (!isDraftInstance && !instanceModelsLoading && instanceModelsSucceeded) {
      onEditedRef.current?.();
    }
  }, [
    instanceItems,
    instanceModelsLoading,
    instanceModelsSucceeded,
    isDraftInstance,
  ]);

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
  instance?: IProviderInstance;
  getFormValues?: () => Record<string, any>;
  verifyTransform?: ModelsSectionProps['verifyTransform'];
}

/**
 * Build the verify call arguments for a single model. Shared by
 * per-model `handleVerify` and batch `handleBatchVerify` so both paths
 * use identical credential resolution logic.
 */
function buildVerifyArgs(
  model: IProviderModelItem,
  providerName: string,
  resolveCreds: () => ResolvedCreds,
  instance: IProviderInstance | undefined,
  getFormValues: (() => Record<string, any>) | undefined,
  verifyTransform: UseModelVerifyArgs['verifyTransform'],
) {
  const modelInfo: IModelInfo[] = [
    {
      model_name: model.name,
      model_type: model.model_types ?? [],
      max_tokens: model.max_tokens ?? 0,
    },
  ];

  let apiKey: string | object;
  let baseUrl: string | undefined;
  let region: string | undefined;

  if (verifyTransform) {
    const transformed = verifyTransform(getFormValues?.() ?? {});
    apiKey = transformed.apiKey;
    baseUrl = transformed.baseUrl;
    region = transformed.region;
  } else {
    const creds = resolveCreds();
    apiKey = creds.apiKey;
    baseUrl = creds.baseUrl;
  }

  // `api_key` is typed `string` on the service signature, but
  // providers with a `verifyTransform` may legitimately produce an
  // object (e.g. PaddleOCR's nested config). The backend accepts
  // both shapes, so cast to `any` to match the existing card-level
  // verify path in `useVerifyProvider`.
  return {
    provider_name: providerName,
    api_key: apiKey as any,
    base_url: baseUrl,
    model_info: modelInfo,
    ...(region ? { region } : {}),
    ...(instance?.id ? { instance_id: instance.id } : {}),
  };
}

export function useModelVerify({
  providerName,
  resolveCreds,
  instanceModels,
  instance,
  getFormValues,
  verifyTransform,
}: UseModelVerifyArgs) {
  const { verifyProviderConnection } = useVerifyProviderConnection();
  const [verify, setVerify] = useState<Record<string, VerifyStatus>>({});
  const [batchVerifying, setBatchVerifying] = useState(false);

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
      const ret = await verifyProviderConnection(
        buildVerifyArgs(
          model,
          providerName,
          resolveCreds,
          instance,
          getFormValues,
          verifyTransform,
        ),
      );
      setVerify((prev) => ({
        ...prev,
        [model.name]: ret.code === 0 ? 'success' : 'error',
      }));
    } catch {
      setVerify((prev) => ({ ...prev, [model.name]: 'error' }));
    }
  };

  // Batch verify: loop through the given models in chunks of 3 parallel
  // requests, suppressing the global error toast for each call. Per-model
  // verify status is updated on the same map used by `handleVerify`.
  const handleBatchVerify = async (models: IProviderModelItem[]) => {
    if (models.length === 0) return;
    setBatchVerifying(true);

    const CHUNK_SIZE = 3;
    for (let i = 0; i < models.length; i += CHUNK_SIZE) {
      const chunk = models.slice(i, i + CHUNK_SIZE);

      // Mark only the current chunk as loading; the remaining models
      // stay idle until their chunk is reached.
      const loadingMap: Record<string, VerifyStatus> = {};
      chunk.forEach((m) => {
        loadingMap[m.name] = 'loading';
      });
      setVerify((prev) => ({ ...prev, ...loadingMap }));

      const results = await Promise.allSettled(
        chunk.map(async (model) => {
          const args = buildVerifyArgs(
            model,
            providerName,
            resolveCreds,
            instance,
            getFormValues,
            verifyTransform,
          );
          // Pass `provider_name` at the top level so the URL function
          // can build `/providers/{provider_name}/connection`; the body
          // goes in `data`. `skipGlobalErrorNotification` suppresses the
          // global toast for each call during batch verify.
          const { data } = await llmService.verifyProviderConnection(
            {
              provider_name: providerName,
              data: args,
              skipGlobalErrorNotification: true,
            },
            true,
          );
          return { name: model.name, code: data?.code ?? -1 };
        }),
      );

      const statusUpdate: Record<string, VerifyStatus> = {};
      results.forEach((result, idx) => {
        const modelName = chunk[idx].name;
        statusUpdate[modelName] =
          result.status === 'fulfilled' && result.value.code === 0
            ? 'success'
            : 'error';
      });
      setVerify((prev) => ({ ...prev, ...statusUpdate }));
    }

    setBatchVerifying(false);
  };

  return { verify, handleVerify, batchVerifying, handleBatchVerify };
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
  clearCatalogOverride: (name: string) => void;
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
  clearCatalogOverride,
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
      clearCatalogOverride(model.name);
      return;
    }
    await addInstanceModel({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: model.name,
      model_type: model.model_types ?? [],
      max_tokens: model.max_tokens ?? 0,
      extra: {
        is_tools: hasToolFeature(model.features),
        ...(model.extra ?? {}),
      },
    });
    clearCatalogOverride(model.name);
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
        clearCatalogOverride(item.name);
      }
      return;
    }
    await addInstanceModel({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: item.name,
      model_type: item.model_types ?? [],
      max_tokens: item.max_tokens ?? 0,
      extra: { is_tools: hasToolFeature(item.features), ...(item.extra ?? {}) },
    });
    clearCatalogOverride(item.name);
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
      filteredModels.forEach((m) => {
        if (!addedSet.has(m.name)) {
          clearCatalogOverride(m.name);
        }
      });
      return;
    }

    const { apiKey, baseUrl } = resolveCreds();
    await updateProviderInstance({
      provider_name: providerName,
      id: instance!.id,
      instance_name: instanceName,
      api_key: apiKey,
      base_url: baseUrl,
      region: instance?.region ?? 'default',
      model_info: buildModelInfo(nextModels),
    });
    filteredModels.forEach((m) => {
      if (!addedSet.has(m.name)) {
        clearCatalogOverride(m.name);
      }
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
  addedSet: Set<string>;
  isDraftInstance?: boolean;
  updateCatalogModel: (name: string, item: IProviderModelItem) => void;
  clearCatalogOverride: (name: string) => void;
  updateDraftModel?: (item: IProviderModelItem) => void;
}

export function useModelEdit({
  providerName,
  instanceName,
  addedSet,
  isDraftInstance,
  updateCatalogModel,
  clearCatalogOverride,
  updateDraftModel,
}: UseModelEditArgs) {
  const queryClient = useQueryClient();
  const customModelDialogFields = useCustomModelFields(providerName);
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

  // Whitelist of provider-specific feature keys derived from the
  // `features` switch-group options. Any option value that is not
  // `is_tools` is treated as provider-specific: on submit it is moved
  // from `features` to `extra` as a boolean; on echo it is converted
  // back from an `extra` boolean to a features array entry.
  const providerFeatureKeys = useMemo(() => {
    const featuresField = customModelDialogFields.find(
      (f) => f.name === 'features',
    );
    return (featuresField?.options ?? [])
      .filter((o) => o.value !== 'is_tools')
      .map((o) => o.value);
  }, [customModelDialogFields]);

  // Initial form values for the edit dialog, derived from the model's
  // persisted `extra` state. The `features` switch-group shows
  // enabled/disabled state, so it must be built from `extra` booleans
  // rather than from `editingModel.features` which merges in
  // catalog-supported features and would incorrectly pre-select
  // features the user has disabled.
  const editDefaultValues = useMemo(() => {
    if (!editingModel) return undefined;
    const extra = editingModel.extra ?? {};
    // Build the features array from `extra` booleans whose keys match
    // the standard feature (`is_tools`) or the provider-specific
    // whitelist. Only `true` values become selected switch-group entries.
    const featureKeySet = new Set<string>(['is_tools', ...providerFeatureKeys]);
    const features: string[] = [];
    const featureBooleans = new Set<string>();
    for (const [key, value] of Object.entries(extra)) {
      if (featureKeySet.has(key) && typeof value === 'boolean') {
        featureBooleans.add(key);
        if (value === true) {
          features.push(key);
        }
      }
    }
    // Remaining extra fields (non-feature: element-format selects, etc.).
    const remainingExtra = Object.fromEntries(
      Object.entries(extra).filter(([k]) => !featureBooleans.has(k)),
    );
    return {
      name: editingModel.name,
      model_types: editingModel.model_types ?? [],
      max_tokens: editingModel.max_tokens ?? 0,
      features,
      ...remainingExtra,
    };
  }, [editingModel, providerFeatureKeys]);

  // Persist edits to an existing model. For drafts the backend has no
  // instance yet, so we update the local `draftModels` list instead of
  // calling PATCH. For saved cards the cache changes only after the backend
  // accepts the edit; the PATCH hook then refetches the complete snapshot.
  const handleEditSubmit = async (item: IProviderModelItem) => {
    if (!editingModel) return;
    const targetName = editingModel.name;

    if (isDraftInstance && updateDraftModel && addedSet.has(targetName)) {
      updateDraftModel(item);
      setEditingModel(null);
      return;
    }

    if (!addedSet.has(targetName)) {
      updateCatalogModel(targetName, item);
      setEditingModel(null);
      return;
    }

    const data = await patchInstanceModel({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: targetName,
      max_tokens: item.max_tokens ?? 0,
      model_type: item.model_types ?? [],
      extra: { is_tools: hasToolFeature(item.features), ...(item.extra ?? {}) },
    });
    if (data.code === 0) {
      queryClient.setQueryData<IInstanceModel[]>(
        LlmKeys.instanceModels(providerName, instanceName),
        (prev) => {
          if (!prev) return prev;
          const idx = prev.findIndex((m) => m.name === targetName);
          if (idx === -1) return prev;
          const next = [...prev];
          const existing = next[idx];
          next[idx] = {
            ...existing,
            max_tokens: item.max_tokens ?? 0,
            model_type: item.model_types ?? [],
            is_tools: hasToolFeature(item.features),
            extra: {
              is_tools: hasToolFeature(item.features),
              ...(item.extra ?? {}),
            },
          };
          return next;
        },
      );
    }
    clearCatalogOverride(targetName);
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
    providerFeatureKeys,
  };
}
