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
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import {
  useAddInstanceModel,
  useDeleteInstanceModels,
  useFetchInstanceModels,
  useListProviderModels,
  useUpdateProviderInstance,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { IInstanceModel, IProviderInstance } from '@/interfaces/database/llm';
import { IModelInfo, IProviderModelItem } from '@/interfaces/request/llm';
import { cn } from '@/lib/utils';
import {
  Check,
  Loader2,
  Minus,
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

type VerifyState =
  | { status: 'idle' }
  | { status: 'loading'; modelName: string }
  | { status: 'success'; modelName: string }
  | { status: 'error'; modelName: string };

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
  const [verify, setVerify] = useState<VerifyState>({ status: 'idle' });
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
    onBlurSuppressChange?.(dialogOpen);
    // On unmount, release any active suppression so the host is not
    // left permanently muted.
    return () => {
      if (dialogOpen) onBlurSuppressChange?.(false);
    };
  }, [dialogOpen, onBlurSuppressChange]);

  // Mark `hasFetched` true once the per-instance query resolves — even if
  // it returned an empty array — so `hideIfEmpty` can safely take effect.
  useEffect(() => {
    if (instanceModels) {
      setHasFetched(true);
    }
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

  // Normalize instance-model records (which historically use `model_type`
  // singular and lack `features`) into the shared `IProviderModelItem`
  // shape used by the catalog. The current backend already returns the
  // newer shape (`model_types` plural, `features`), but we defensively
  // handle both.
  const instanceItems: IProviderModelItem[] = useMemo(() => {
    return (instanceModels ?? []).map((im: any) => {
      const rawTypes = im.model_types ?? im.model_type ?? [];
      const model_types: string[] = Array.isArray(rawTypes)
        ? rawTypes
        : rawTypes
          ? [rawTypes]
          : [];
      return {
        name: im.name,
        max_tokens: im.max_tokens ?? 0,
        model_types,
        features: im.features ?? null,
      };
    });
  }, [instanceModels]);

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
    await addInstanceModel({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: item.name,
      model_type: item.model_types ?? [],
      max_tokens: item.max_tokens ?? 0,
    });
  };

  const handleVerify = async (model: IProviderModelItem) => {
    setVerify({ status: 'loading', modelName: model.name });
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
      setVerify({
        status: ret.code === 0 ? 'success' : 'error',
        modelName: model.name,
      });
    } catch {
      setVerify({ status: 'error', modelName: model.name });
    }
  };

  const handleAddModel = async (model: IProviderModelItem) => {
    await addInstanceModel({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: model.name,
      model_type: model.model_types ?? [],
      max_tokens: model.max_tokens ?? 0,
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
      if (m.features && m.features.length) {
        info.extra = {
          is_tools: m.features.some((f) =>
            ['tool_call', 'tools', 'function_call'].includes(f),
          ),
        };
      }
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
                <Minus className="size-4" />
              ) : (
                <Plus className="size-4" />
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
              const verifyStatus =
                'modelName' in verify && verify.modelName === model.name
                  ? verify.status
                  : 'idle';
              return (
                <li
                  key={model.name}
                  className="flex items-center justify-between gap-3 p-3 border-b border-border-button last:border-b-0 hover:bg-bg-input transition-colors"
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
                    <div className="flex flex-wrap gap-1">
                      {(model.model_types ?? []).map((mt) => (
                        <span
                          key={mt}
                          className="px-1.5 py-0.5 text-[10px] bg-bg-card text-text-secondary rounded-md"
                        >
                          {mapModelKey[mt as keyof typeof mapModelKey] || mt}
                        </span>
                      ))}
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
                        verifyStatus === 'error' && 'text-state-error',
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
                          'size-6 flex items-center justify-center rounded-md transition-colors',
                          isAdded
                            ? 'text-state-error hover:bg-state-error/10'
                            : 'text-state-success hover:bg-state-success/10',
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
    </div>
  );
}

export default ModelsSection;
