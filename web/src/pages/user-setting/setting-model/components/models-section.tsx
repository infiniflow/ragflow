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
import { ModelStatus } from '@/constants/llm';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import {
  useAddInstanceModel,
  useFetchInstanceModels,
  useListProviderModels,
  useUpdateModelStatus,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { IInstanceModel, IProviderInstance } from '@/interfaces/database/llm';
import { IProviderModelItem } from '@/interfaces/request/llm';
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
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AddCustomModelDialog } from '../model/provider-model/components/add-custom-model-dialog';
import { useCustomModelFields } from '../model/provider-model/components/use-custom-model-fields';
import { mapModelKey, sortModelTypes } from './un-add-model';

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
 *  - "-" calls `updateModelStatus(... Inactive)`. There is no dedicated
 *    "delete instance model" endpoint, so flipping to Inactive is the
 *    de-facto "remove" operation. This is documented in the report.
 */
export function ModelsSection({
  providerName,
  instanceName,
  instance,
  hideActions = false,
  hideIfEmpty = false,
}: ModelsSectionProps) {
  const { t } = useTranslation();
  const { t: tSetting } = useTranslate('setting');
  const { t: tc } = useCommonTranslation();
  const customModelDialogFields = useCustomModelFields();
  const { listProviderModels, loading: listLoading } = useListProviderModels();
  const { addInstanceModel } = useAddInstanceModel();
  const { updateModelStatus } = useUpdateModelStatus();
  const { verifyProviderConnection } = useVerifyProviderConnection();
  const { data: instanceModels } = useFetchInstanceModels(
    providerName,
    instanceName,
  );

  const [models, setModels] = useState<IProviderModelItem[]>([]);
  const [search, setSearch] = useState('');
  const [tag, setTag] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [verify, setVerify] = useState<VerifyState>({ status: 'idle' });
  // Tracks whether the first auto-fetch has completed (success OR fail).
  // Used by `hideIfEmpty` to decide when it is safe to hide the section.
  const [hasFetched, setHasFetched] = useState(false);

  const apiKey = instance?.api_key ?? '';
  const baseUrl = instance?.base_url ?? '';

  // Fetch the model catalog on first mount. We tolerate empty credentials
  // here so the section is usable for new/draft instances — the backend
  // will reject the call but the UI will not crash.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const ret = await listProviderModels({
          provider_name: providerName,
          api_key: apiKey,
          base_url: baseUrl,
        });
        if (!cancelled) {
          if (ret?.code === 0) {
            setModels((ret.data as IProviderModelItem[]) ?? []);
          }
          setHasFetched(true);
        }
      } catch {
        if (!cancelled) {
          setHasFetched(true);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
    // We intentionally do NOT re-run on apiKey/baseUrl changes — those
    // are stable per instance and re-listing on every keystroke would be
    // expensive. Use the manual "List models" button to refresh.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [providerName]);

  // Manual "List models" handler also marks `hasFetched` so `hideIfEmpty`
  // takes effect after a user-triggered refresh as well.
  const handleListModels = async () => {
    try {
      const ret = await listProviderModels({
        provider_name: providerName,
        api_key: apiKey,
        base_url: baseUrl,
      });
      if (ret?.code === 0) {
        setModels((ret.data as IProviderModelItem[]) ?? []);
      }
      setHasFetched(true);
    } catch {
      setHasFetched(true);
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
    setModels((prev) =>
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
    await updateModelStatus({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: model.name,
      status: ModelStatus.Inactive,
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
              disabled={listLoading}
              data-testid="models-list-button"
            >
              {listLoading && <Loader2 className="size-3 animate-spin" />}
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
        <SearchInput
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t('setting.search')}
        />
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
