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

import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import {
  useAddInstanceModel,
  useDeleteInstanceModels,
  useFetchInstanceModels,
  useListProviderModels,
  usePatchInstanceModel,
  useUpdateProviderInstance,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { IInstanceModel, IProviderInstance } from '@/interfaces/database/llm';
import { IModelInfo, IProviderModelItem } from '@/interfaces/request/llm';
import { cn } from '@/lib/utils';
import {
  Check,
  ListMinus,
  ListPlus,
  Loader2,
  Minus,
  Pencil,
  Plus,
  RefreshCcw,
  Search,
  TriangleAlert,
} from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AddCustomModelDialog } from './add-custom-model-dialog';
import { mapModelKey, sortModelTypes } from './available-models';
import { useCustomModelFields } from './use-custom-model-fields';

type VerifyStatus = 'idle' | 'loading' | 'success' | 'error';

interface ModelsSectionProps {
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
}

/**
 * Models sub-section rendered inside each ProviderInstanceCard.
 *
 * Responsibilities:
 *  - Fetch the catalog from the backend via `useListProviderModels` on mount
 *    and on demand via the "List models" button.
 *  - Let the user add custom models via a small dialog (name / max_tokens /
 *    model_types) — these are appended to the local catalog only.
 *  - Render the catalog with search + tag filtering.
 *  - For each model, expose:
 *      * a verify control (idle → spinner → ✓ / !),
 *      * a +/- control reflecting whether the model is already attached to
 *        the instance.
 *
 * The +/- semantics:
 *  - "+" calls `addInstanceModel` (backend persistence).
 *  - "-" calls `deleteInstanceModels` to drop the model from this instance.
 */
export function ModelsSection({
  providerName,
  instanceName,
  instance,
  hideActions = false,
  hideIfEmpty = false,
  getFormValues,
  onBlurSuppressChange,
  onInstanceModelsChange,
}: ModelsSectionProps) {
  const { t } = useTranslation();
  const { t: tSetting } = useTranslate('setting');
  const { t: tc } = useCommonTranslation();
  const customModelDialogFields = useCustomModelFields();
  const { listProviderModels } = useListProviderModels();
  const { addInstanceModel } = useAddInstanceModel();
  const { deleteInstanceModels } = useDeleteInstanceModels();
  const { updateProviderInstance, loading: batchLoading } =
    useUpdateProviderInstance();
  const { patchInstanceModel, loading: editLoading } = usePatchInstanceModel();
  const { verifyProviderConnection } = useVerifyProviderConnection();
  const { data: instanceModels } = useFetchInstanceModels(
    providerName,
    instanceName,
  );

  // `catalog` holds the upstream provider's full model list, populated only
  // when the user explicitly clicks "List Models" or adds a custom model.
  // The displayed `models` below is computed as a union of `catalog` and
  // the per-instance saved models (`instanceModels`), so on mount we show
  // exactly the saved set without any extra network round-trip.
  const [catalog, setCatalog] = useState<IProviderModelItem[]>([]);
  const [search, setSearch] = useState('');
  const [tag, setTag] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  // Model currently being edited via AddCustomModelDialog (with `name`
  // pinned/disabled and the dialog initial values pre-populated from the
  // model's current config). `null` when the edit dialog is closed.
  const [editingModel, setEditingModel] = useState<IProviderModelItem | null>(
    null,
  );
  const [verify, setVerify] = useState<Record<string, VerifyStatus>>({});
  // True only while the user is actively waiting on a click of the
  // "List Models" button. Kept independent from the mutation's own
  // `isPending` flag because the same mutation is also used by the
  // auto-fetch on mount — driving the button spinner from the shared
  // mutation state would make the button appear stuck whenever the
  // background auto-fetch is slow or fails to settle.
  const [manualListLoading, setManualListLoading] = useState(false);

  // True once we have either (a) received instance models from the backend
  // or (b) the user has clicked "List Models". Used by `hideIfEmpty`.
  const [hasFetched, setHasFetched] = useState(false);

  // Resolve the credentials/url used for the catalog and verify calls.
  // Prefer the live values from the host card's form (so the user can
  // verify with an api_key they have just typed but not yet saved); fall
  // back to the persisted instance fields when no form getter is wired up.
  const resolveCreds = () => {
    const fv = getFormValues?.() ?? {};
    return {
      apiKey: (fv.api_key as string) ?? instance?.api_key ?? '',
      baseUrl:
        (fv.base_url as string) ??
        (fv.api_base as string) ??
        instance?.base_url ??
        '',
    };
  };

  // Mirror the AddCustomModelDialog open state up to the host so it can
  // pause its blur-driven auto-save while the dialog is open. The dialog
  // body lives in a React Portal outside the host's `onBlurCapture`
  // container — without this guard, opening the dialog (focus shifts
  // into the Portal) would otherwise fire a spurious save.
  useEffect(() => {
    const open = dialogOpen || editingModel !== null;
    onBlurSuppressChange?.(open);
    // On unmount, release any active suppression so the host is not
    // left permanently muted.
    return () => {
      if (open) onBlurSuppressChange?.(false);
    };
  }, [dialogOpen, editingModel, onBlurSuppressChange]);

  // Mark `hasFetched` true once the per-instance query resolves — even if
  // it returned an empty array — so `hideIfEmpty` can safely take effect.
  useEffect(() => {
    if (instanceModels) {
      setHasFetched(true);
    }
  }, [instanceModels]);

  // Seed the per-model verify status from the backend's persisted
  // `verify` flag on each instance model. `true` → success, `false`
  // → error, `undefined` → not verified yet. Local user-triggered
  // verifications (already present in `verify` state) are preserved:
  // we only fill in models the user has not yet acted on in this
  // session, so a fresh 'success' from `handleVerify` is not clobbered
  // by a stale backend value after cache invalidation.
  useEffect(() => {
    if (!instanceModels || instanceModels.length === 0) return;
    setVerify((prev) => {
      let changed = false;
      const next = { ...prev };
      for (const im of instanceModels) {
        if (im.name in next) continue;
        if (im.verify === true) {
          next[im.name] = 'success';
          changed = true;
        } else if (im.verify === false) {
          next[im.name] = 'error';
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [instanceModels]);

  // Auto-fetch the provider's available-models catalog when this section
  // mounts. Because the host card wraps <ModelsSection> in `{open && ...}`,
  // "on mount" is effectively "when the card is expanded", so this gives
  // us the open-time catalog fetch the page wants. The fired list is then
  // unioned with the per-instance saved models below.
  //
  // Skipped for:
  //   - draft instances (no api_key yet — backend call would fail),
  //   - sections rendered with hideActions (catalog-preview-only host).
  // The `hasAutoFetchedRef` guard ensures we only auto-fetch once per
  // mount, even if props churn before the request settles.
  const hasAutoFetchedRef = useRef(false);
  useEffect(() => {
    if (hasAutoFetchedRef.current) return;
    if (hideActions) return;
    if (!providerName) return;
    if (!instanceName || instanceName === '__draft__') return;
    hasAutoFetchedRef.current = true;
    handleListModels();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [providerName, instanceName, hideActions]);

  const catalogFeatures = useMemo(() => {
    const map = new Map<string, string[]>();
    catalog.forEach((m) => {
      if (Array.isArray(m.features) && m.features.length > 0) {
        map.set(m.name, m.features);
      }
    });
    return map;
  }, [catalog]);

  const instanceItems: IProviderModelItem[] = useMemo(() => {
    return (instanceModels ?? []).map((im: any) => {
      const rawTypes = im.model_types ?? im.model_type ?? [];
      const model_types: string[] = Array.isArray(rawTypes)
        ? rawTypes
        : rawTypes
          ? [rawTypes]
          : [];
      console.log(`[ModelsSection] instanceModel: `, im);
      const catalogFeats = catalogFeatures.get(im.name) ?? im.features ?? null;
      const hasToolFlag = (feats: string[] | null | undefined) =>
        Array.isArray(feats) &&
        feats.some((f) =>
          ['is_tools', 'tool_call', 'tools', 'function_call'].includes(f),
        );
      let features: string[] | null = catalogFeats;
      if (im.is_tools && !hasToolFlag(catalogFeats)) {
        features = [...(catalogFeats ?? []), 'is_tools'];
      }
      return {
        name: im.name,
        max_tokens: im.max_tokens ?? 0,
        model_types,
        features,
      };
    });
  }, [instanceModels, catalogFeatures]);

  // Union of instance models + catalog, keyed by `name`. Catalog entries
  // win on conflict because the upstream list is authoritative for
  // `features` / `max_tokens`. The instance set is listed first so that
  // already-added models stay at the top of the list on the initial
  // render before the catalog is fetched.
  const models: IProviderModelItem[] = useMemo(() => {
    const byName = new Map<string, IProviderModelItem>();
    instanceItems.forEach((m) => byName.set(m.name, m));
    catalog.forEach((m) => byName.set(m.name, m));
    return Array.from(byName.values());
  }, [instanceItems, catalog]);

  // Push the latest per-instance model list up to the host so its
  // auto-save (draft watch-effect or saved-instance blur handler) can
  // include `model_info` in the payload. The host caches the value in
  // a ref, so it always reads the freshest list when its own debounce
  // / blur fires — even after add/remove mutations that invalidate
  // `useFetchInstanceModels`.
  useEffect(() => {
    console.log(`[ModelsSection] onInstanceModelsChange: `, instanceItems);
    onInstanceModelsChange?.(toModelInfo(instanceItems));
  }, [instanceItems, onInstanceModelsChange]);

  // Manual "List models" handler — still hits the upstream catalog
  // endpoint. The result is merged into `catalog`; the displayed list
  // then becomes the union of catalog + instance models.
  const handleListModels = async () => {
    setManualListLoading(true);
    try {
      const { apiKey, baseUrl } = resolveCreds();
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

  const allTags = useMemo(() => {
    const tagsSet = new Set<string>();
    models.forEach((m) => m.model_types?.forEach((t) => tagsSet.add(t)));
    return sortModelTypes(Array.from(tagsSet));
  }, [models]);

  const filteredModels = useMemo(() => {
    const q = search.trim().toLowerCase();
    return models.filter((m) => {
      if (q && !m.name.toLowerCase().includes(q)) return false;
      if (tag && !m.model_types?.includes(tag)) return false;
      return true;
    });
  }, [models, search, tag]);

  const addedSet = useMemo(() => {
    return new Set(instanceModels.map((m: IInstanceModel) => m.name));
  }, [instanceModels]);

  const handleAddCustom = async (item: IProviderModelItem) => {
    // Append the custom item to the local catalog so it shows up in the
    // unioned `models` list immediately. Server-side persistence happens
    // via `addInstanceModel` below (when there is a real instance).
    setCatalog((prev) =>
      prev.some((m) => m.name === item.name) ? prev : [...prev, item],
    );
    // Persist the new model to the current instance. Skip the call only
    // when there is no real instance yet (draft placeholder uses the
    // `__draft__` sentinel) or when the host has explicitly hidden the
    // model actions (so the user is just previewing the catalog).
    if (hideActions || !instanceName || instanceName === '__draft__') {
      return;
    }
    // Derive `is_tools` from the features switch group in the dialog
    // (same key list as `handleEditSubmit` / `toModelInfo`). Forwarded
    // via `extra` so the backend can persist the Tool-call flag on the
    // freshly-added custom model.
    const isTools =
      Array.isArray(item.features) &&
      item.features.some((f) =>
        ['is_tools', 'tool_call', 'tools', 'function_call'].includes(f),
      );
    await addInstanceModel({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: item.name,
      model_type: item.model_types ?? [],
      max_tokens: item.max_tokens ?? 0,
      extra: { is_tools: isTools },
    });
  };

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

  const handleAddModel = async (model: IProviderModelItem) => {
    // Derive `is_tools` from the model's features (same key list used
    // by `handleAddCustom` / `handleEditSubmit` / `toModelInfo`) so the
    // Tool-call flag survives the round-trip when a user attaches an
    // existing catalog model via the +/- button.
    const isTools =
      Array.isArray(model.features) &&
      model.features.some((f) =>
        ['is_tools', 'tool_call', 'tools', 'function_call'].includes(f),
      );
    await addInstanceModel({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: model.name,
      model_type: model.model_types ?? [],
      max_tokens: model.max_tokens ?? 0,
      extra: { is_tools: isTools },
    });
  };

  const handleRemoveModel = async (model: IProviderModelItem) => {
    await deleteInstanceModels({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: [model.name],
    });
  };

  // Build an IModelInfo array (the shape required by the PUT
  // `/providers/{name}/instances/{name}` endpoint) from a list of
  // provider model items. `features` is forwarded via `extra` so the
  // backend can persist per-model flags such as `is_tools`.
  const toModelInfo = (items: IProviderModelItem[]): IModelInfo[] =>
    items.map((m) => {
      const info: IModelInfo = {
        model_name: m.name,
        model_type: m.model_types ?? [],
        max_tokens: m.max_tokens ?? 0,
      };
      // Feature-key list kept in sync with `handleAddCustom`,
      // `handleAddModel`, and `handleEditSubmit`.
      const isTools =
        Array.isArray(m.features) &&
        m.features.some((f) =>
          ['is_tools', 'tool_call', 'tools', 'function_call'].includes(f),
        );
      info.extra = { is_tools: isTools };
      return info;
    });

  // True when every model currently shown in the filtered list is
  // already attached to the instance — drives the +/- toggle on the
  // batch button. Vacuously false when no models are visible so the
  // button does not render as a "remove" when there is nothing to
  // remove.
  const allFilteredAdded = useMemo(() => {
    return (
      filteredModels.length > 0 &&
      filteredModels.every((m) => addedSet.has(m.name))
    );
  }, [filteredModels, addedSet]);

  /**
   * Batch attach/detach the currently visible (filtered) models to the
   * instance via the PUT `/providers/{name}/instances/{name}` endpoint.
   * The endpoint replaces `model_info` wholesale, so we always compute
   * the full next set:
   *   - batch add → union(existing instance models, visible models)
   *   - batch remove → existing instance models minus visible models
   * Models that are attached but not in the current filtered view are
   * preserved either way.
   */
  const handleBatchToggleModels = async () => {
    if (filteredModels.length === 0) return;
    const { apiKey, baseUrl } = resolveCreds();

    // Keyed view of the existing per-instance set so we can compute
    // the next set without losing fields (max_tokens, model_types, ...).
    const byName = new Map<string, IProviderModelItem>();
    instanceItems.forEach((m) => byName.set(m.name, m));

    let nextModels: IProviderModelItem[];
    if (allFilteredAdded) {
      // Remove every filtered model from the existing set.
      const drop = new Set(filteredModels.map((m) => m.name));
      nextModels = Array.from(byName.values()).filter((m) => !drop.has(m.name));
    } else {
      // Add every filtered model on top of the existing set; catalog
      // entry wins on conflict (it carries authoritative features /
      // max_tokens).
      filteredModels.forEach((m) => byName.set(m.name, m));
      nextModels = Array.from(byName.values());
    }

    await updateProviderInstance({
      provider_name: providerName,
      instance_name: instanceName,
      api_key: apiKey,
      base_url: baseUrl,
      region: instance?.region ?? 'default',
      model_info: toModelInfo(nextModels),
    });
  };

  // Field schema for the edit dialog — identical to the add schema
  // except the `name` field is locked (model name is the row's primary
  // key and the API forbids renaming via this endpoint).
  const editModelDialogFields = useMemo(() => {
    return customModelDialogFields.map((f) =>
      f.name === 'name' ? { ...f, disabled: true } : f,
    );
  }, [customModelDialogFields]);

  // Initial form values for the edit dialog, derived from the model
  // currently being edited. `features` defaults to [] so the
  // switch-group renders unchecked when the model has no flags.
  const editDefaultValues = useMemo(() => {
    if (!editingModel) return undefined;
    return {
      name: editingModel.name,
      model_types: editingModel.model_types ?? [],
      max_tokens: editingModel.max_tokens ?? 0,
      features: editingModel.features ?? [],
    };
  }, [editingModel]);

  /**
   * Persist edits to an existing model. The PUT endpoint replaces the
   * full `model_info` set, so we splice the edited model into the
   * current instance's list (or append if it was only in the catalog,
   * which effectively converts an edit on a draft into an attach).
   * The local `catalog` is also patched so the UI reflects the new
   * values immediately, before the cache invalidation lands.
   */
  const handleEditSubmit = async (item: IProviderModelItem) => {
    if (!editingModel) return;
    const targetName = editingModel.name;

    // Patch the local catalog so any catalog-only row reflects the edit
    // even before the per-instance refetch settles.
    setCatalog((prev) =>
      prev.some((m) => m.name === targetName)
        ? prev.map((m) => (m.name === targetName ? item : m))
        : prev,
    );

    // Derive is_tools from the features switch group. `null`/empty means
    // the user has no flags selected for this model.
    const isTools =
      Array.isArray(item.features) &&
      item.features.some((f) =>
        ['is_tools', 'tool_call', 'tools', 'function_call'].includes(f),
      );

    await patchInstanceModel({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: targetName,
      max_tokens: item.max_tokens ?? 0,
      model_type: item.model_types ?? [],
      extra: { is_tools: isTools },
    });
    setEditingModel(null);
  };

  // When `hideIfEmpty` is set and the first fetch has completed with
  // zero models, render nothing. This is used by draft instances so
  // an unsuccessful (or rejected) `listProviderModels` call does not
  // leave a noisy empty list on screen.
  if (hideIfEmpty && hasFetched && models.length === 0) {
    return null;
  }

  return (
    <div className="flex flex-col gap-3" data-testid="models-section">
      <div className="flex items-center justify-between gap-2">
        <div className="text-sm font-medium text-text-primary">
          {t('setting.models')}
        </div>
        {!hideActions && (
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={handleListModels}
              disabled={manualListLoading}
              data-testid="models-list-button"
            >
              {manualListLoading && <Loader2 className="size-3 animate-spin" />}
              {t('setting.listModels')}
            </Button>
            <Button
              variant="outline"
              size="icon-sm"
              onClick={() => setDialogOpen(true)}
              data-testid="models-add-custom"
              aria-label={t('setting.addCustomModel')}
            >
              <Plus className="size-4" />
            </Button>
          </div>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <SearchInput
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('setting.search')}
            rootClassName="flex-1"
          />
          {!hideActions && (
            <Button
              variant="outline"
              size="icon-sm"
              onClick={handleBatchToggleModels}
              disabled={batchLoading || filteredModels.length === 0}
              data-testid="models-batch-toggle"
              aria-label={
                allFilteredAdded
                  ? tSetting('batchRemoveModels')
                  : tSetting('batchAddModels')
              }
              title={
                allFilteredAdded
                  ? tSetting('batchRemoveModels')
                  : tSetting('batchAddModels')
              }
            >
              {batchLoading ? (
                <Loader2 className="size-4 animate-spin" />
              ) : allFilteredAdded ? (
                <ListMinus className="size-4" />
              ) : (
                <ListPlus className="size-4" />
              )}
            </Button>
          )}
        </div>
        <div className="flex flex-wrap gap-1.5">
          <button
            type="button"
            className={cn(
              'px-2 py-0.5 text-xs rounded-md border border-border-button transition-colors',
              tag === null
                ? 'bg-text-primary text-bg-base'
                : 'bg-bg-card text-text-secondary hover:text-text-primary',
            )}
            onClick={() => setTag(null)}
          >
            {tSetting('allModels')}
            <span className="ml-1 opacity-60">{models.length}</span>
          </button>
          {allTags.map((tKey) => (
            <button
              key={tKey}
              type="button"
              className={cn(
                'px-2 py-0.5 text-xs rounded-md border border-border-button transition-colors',
                tag === tKey
                  ? 'bg-text-primary text-bg-base'
                  : 'bg-bg-card text-text-secondary hover:text-text-primary',
              )}
              onClick={() => setTag(tag === tKey ? null : tKey)}
            >
              {mapModelKey[tKey as keyof typeof mapModelKey] || tKey}
              <span className="ml-1 opacity-60">
                {models.filter((m) => m.model_types?.includes(tKey)).length}
              </span>
            </button>
          ))}
        </div>
      </div>

      <div className="bg-bg-card rounded-lg max-h-80 overflow-auto scrollbar-auto border border-border-button">
        {filteredModels.length === 0 ? (
          <div className="flex items-center justify-center text-text-secondary text-sm py-6 gap-2">
            <Search className="size-4" />
            {t('setting.listModelsEmpty')}
          </div>
        ) : (
          <ul>
            {filteredModels.map((model) => {
              const isAdded = addedSet.has(model.name);
              const verifyStatus: VerifyStatus = verify[model.name] ?? 'idle';
              return (
                <li
                  key={model.name}
                  className="group flex items-center justify-between gap-3 p-3 border-b border-border-button last:border-b-0 hover:bg-bg-input transition-colors"
                  data-testid={`models-row-${model.name}`}
                >
                  <div className="flex gap-1 min-w-0">
                    <div className="flex items-center gap-2 min-w-0">
                      <span className="font-medium text-sm text-text-primary truncate">
                        {model.name}
                      </span>
                      {/* <span className="text-xs text-text-secondary">
                        {model.max_tokens ?? 0}
                      </span> */}
                    </div>
                    <div className="flex flex-wrap items-center gap-1">
                      {(() => {
                        const types = model.model_types ?? [];
                        const visible = types.slice(0, 3);
                        const hidden = types.slice(3);
                        return (
                          <>
                            {visible.map((mt) => (
                              <span
                                key={mt}
                                className="px-1.5 py-0.5 text-[10px] bg-bg-card text-text-secondary rounded-md"
                              >
                                {mapModelKey[mt as keyof typeof mapModelKey] ||
                                  mt}
                              </span>
                            ))}
                            {hidden.length > 0 && (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <span
                                    className="px-1.5 py-0.5 text-[10px] bg-bg-card text-text-secondary rounded-md cursor-default"
                                    data-testid={`models-types-overflow-${model.name}`}
                                  >
                                    +{hidden.length}
                                  </span>
                                </TooltipTrigger>
                                <TooltipContent>
                                  <div className="flex flex-wrap gap-1 max-w-[16rem]">
                                    {hidden.map((mt) => (
                                      <span
                                        key={mt}
                                        className="px-1.5 py-0.5 text-[10px] bg-bg-card text-text-secondary rounded-md"
                                      >
                                        {mapModelKey[
                                          mt as keyof typeof mapModelKey
                                        ] || mt}
                                      </span>
                                    ))}
                                  </div>
                                </TooltipContent>
                              </Tooltip>
                            )}
                            {!hideActions && (
                              <button
                                type="button"
                                className="ml-1 size-5 flex items-center justify-center rounded-md text-text-secondary opacity-0 transition-all hover:bg-bg-card hover:text-text-primary group-hover:opacity-100 focus-visible:opacity-100"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  setEditingModel(model);
                                }}
                                aria-label={tSetting('editModel')}
                                title={tSetting('editModel')}
                                data-testid={`models-edit-${model.name}`}
                              >
                                <Pencil className="size-3" />
                              </button>
                            )}
                          </>
                        );
                      })()}
                    </div>
                  </div>

                  <div className="flex items-center gap-2 shrink-0">
                    <button
                      type="button"
                      className={cn(
                        'size-6 flex items-center justify-center rounded-md transition-colors',
                        verifyStatus === 'idle' &&
                          'text-text-secondary hover:bg-bg-input hover:text-text-primary',
                        verifyStatus === 'loading' &&
                          'text-text-secondary cursor-wait',
                        verifyStatus === 'success' && 'text-state-success',
                        verifyStatus === 'error' && 'text-state-warning',
                      )}
                      onClick={() => handleVerify(model)}
                      disabled={verifyStatus === 'loading'}
                      aria-label={`Verify ${model.name}`}
                    >
                      {verifyStatus === 'loading' && (
                        <Loader2 className="size-4 animate-spin" />
                      )}
                      {verifyStatus === 'success' && (
                        <Check className="size-4" />
                      )}
                      {verifyStatus === 'error' && (
                        <TriangleAlert className="size-4" />
                      )}
                      {verifyStatus === 'idle' && (
                        <RefreshCcw className="size-3" />
                      )}
                    </button>

                    {!hideActions && (
                      <button
                        type="button"
                        className={cn(
                          'size-6 flex items-center justify-center rounded-md transition-colors text-text-secondary',
                        )}
                        onClick={() =>
                          isAdded
                            ? handleRemoveModel(model)
                            : handleAddModel(model)
                        }
                        aria-label={
                          isAdded ? `Remove ${model.name}` : `Add ${model.name}`
                        }
                      >
                        {isAdded ? (
                          <Minus className="size-4" />
                        ) : (
                          <Plus className="size-4" />
                        )}
                      </button>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <AddCustomModelDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={tSetting('addCustomModelTitle')}
        fields={customModelDialogFields}
        existingNames={models.map((m) => m.name)}
        onSubmit={async (item) => {
          await handleAddCustom(item);
          setDialogOpen(false);
        }}
        submitText={tc('confirm')}
        cancelText={tc('cancel')}
      />

      <AddCustomModelDialog
        open={editingModel !== null}
        onOpenChange={(open) => {
          if (!open) setEditingModel(null);
        }}
        title={tSetting('editModel')}
        fields={editModelDialogFields}
        // Exclude the model being edited from the uniqueness check so
        // submitting the unchanged (disabled) name does not flag a
        // duplicate against itself.
        existingNames={models
          .filter((m) => m.name !== editingModel?.name)
          .map((m) => m.name)}
        defaultValues={editDefaultValues}
        loading={editLoading}
        onSubmit={async (item) => {
          await handleEditSubmit(item);
        }}
        submitText={tc('confirm')}
        cancelText={tc('cancel')}
      />
    </div>
  );
}

export default ModelsSection;
