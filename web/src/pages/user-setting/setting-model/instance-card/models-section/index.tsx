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

import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import { useFetchInstanceModels } from '@/hooks/use-llm-request';
import { IProviderModelItem } from '@/interfaces/request/llm';
import { Loader2, Plus, Search, ShieldCheck } from 'lucide-react';
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { AddCustomModelDialog } from '../add-custom-model-dialog';
import { mapModelKey } from '../available-models';
import { ModelRow } from './components/model-row';
import { TagFilterButton } from './components/tag-filter-button';
import {
  DRAFT_INSTANCE_SENTINEL,
  useModelEdit,
  useModelMutations,
  useModelVerify,
  useModelsCatalog,
  useModelsDerived,
  useModelsFilter,
  useResolveCreds,
} from './hooks';
import { ModelsSectionProps } from './interface';

export function ModelsSection(props: ModelsSectionProps) {
  const { t } = useTranslation();
  const { t: tSetting } = useTranslate('setting');
  const { t: tc } = useCommonTranslation();

  const {
    providerName,
    instanceName,
    instance,
    hideActions = false,
    hideIfEmpty = false,
    getFormValues,
    verifyTransform,
    instanceDetailsLoaded,
    onBlurSuppressChange,
    onInstanceModelsChange,
    onInstanceModelsEdited,
    onInstanceModelsStatusChange,
  } = props;

  const isDraftInstance =
    !instanceName || instanceName === DRAFT_INSTANCE_SENTINEL;

  // 1. Credentials for catalog / verify / batch calls.
  const { resolveCreds } = useResolveCreds(instance, getFormValues);

  // Snapshot of the current api_key so `useModelsCatalog` can gate the
  // auto-fetch for VolcEngine on the user actually having typed one.
  // Recomputed on every render so the effect re-runs as soon as the
  // form value lands.
  const currentCreds = resolveCreds();

  // 2. Per-instance saved models (shared by catalog, derived, verify).
  const {
    data: instanceModels,
    loading: instanceModelsLoading,
    isSuccess: instanceModelsSucceeded,
  } = useFetchInstanceModels(providerName, instanceName);

  useLayoutEffect(() => {
    if (
      !isDraftInstance &&
      (instanceModelsLoading || !instanceModelsSucceeded)
    ) {
      onInstanceModelsStatusChange?.(false);
    }
  }, [
    instanceModelsLoading,
    instanceModelsSucceeded,
    isDraftInstance,
    onInstanceModelsStatusChange,
  ]);

  // 3a. Draft-only: locally-tracked "added models" list.
  // The backend has no per-instance models yet, so per-model add /
  // remove / batch-toggle on a draft mutates this array instead of
  // firing a mutation. The host save handler then flushes the latest
  // snapshot through `model_info` on save. Reset when the provider
  // or instance changes (rare in practice since the host remounts
  // the section on draft switch, but kept as a safety net).
  const [draftModels, setDraftModels] = useState<IProviderModelItem[]>([]);
  // Names of catalog models the user has manually removed for this draft.
  // Every catalog fetch (mount auto-fetch and "List models" clicks)
  // auto-adds incoming models EXCEPT the names in this set, so removed
  // models stay removed while newly listed models are added by default.
  const removedDraftModelsRef = useRef<Set<string>>(new Set());
  useEffect(() => {
    setDraftModels([]);
    removedDraftModelsRef.current = new Set();
  }, [providerName, instanceName]);

  // Merge a freshly fetched catalog batch into the draft list, skipping
  // already-added models and ones the user has previously removed.
  const mergeCatalogIntoDraft = useCallback((items: IProviderModelItem[]) => {
    const removed = removedDraftModelsRef.current;
    setDraftModels((prev) => {
      const existing = new Set(prev.map((m) => m.name));
      const incoming = items.filter(
        (m) => !existing.has(m.name) && !removed.has(m.name),
      );
      return incoming.length === 0 ? prev : [...prev, ...incoming];
    });
  }, []);

  // 3. Upstream catalog + auto-fetch on mount.
  const {
    catalog,
    setCatalog,
    updateCatalogModel,
    clearCatalogOverride,
    manualListLoading,
    hasFetched,
    handleListModels,
  } = useModelsCatalog({
    providerName,
    instanceName,
    hideActions,
    resolveCreds,
    instanceModels,
    apiKeyValue: currentCreds.apiKey,
    baseUrlValue: currentCreds.baseUrl,
    instanceDetailsLoaded,
    onCatalogFetched: isDraftInstance ? mergeCatalogIntoDraft : undefined,
  });

  const addDraftModel = useCallback((model: IProviderModelItem) => {
    removedDraftModelsRef.current.delete(model.name);
    setDraftModels((prev) =>
      prev.some((m) => m.name === model.name) ? prev : [...prev, model],
    );
  }, []);
  const removeDraftModel = useCallback((name: string) => {
    removedDraftModelsRef.current.add(name);
    setDraftModels((prev) => prev.filter((m) => m.name !== name));
  }, []);
  const updateDraftModel = useCallback((item: IProviderModelItem) => {
    setDraftModels((prev) =>
      prev.map((m) => (m.name === item.name ? { ...m, ...item } : m)),
    );
  }, []);
  // Batch toggle replaces the whole list. Diff the next list against the
  // current one so batch-removed names stay excluded from future catalog
  // merges and batch-added names are eligible again.
  const applyDraftModelsList = useCallback(
    (next: IProviderModelItem[]) => {
      const removed = removedDraftModelsRef.current;
      setDraftModels((prev) => {
        const nextNames = new Set(next.map((m) => m.name));
        prev.forEach((m) => {
          if (!nextNames.has(m.name)) removed.add(m.name);
        });
        next.forEach((m) => removed.delete(m.name));
        return next;
      });
    },
    [],
  );

  // 4. Derived union list (instance ∪ catalog) + push to host.
  const { instanceItems, models, addedSet } = useModelsDerived({
    catalog,
    instanceModels,
    instanceModelsLoading,
    instanceModelsSucceeded,
    draftModels,
    isDraftInstance,
    onInstanceModelsChange,
    onInstanceModelsEdited,
  });

  useEffect(() => {
    if (!isDraftInstance && !instanceModelsLoading && instanceModelsSucceeded) {
      onInstanceModelsStatusChange?.(true);
    }
  }, [
    instanceModelsLoading,
    instanceModelsSucceeded,
    isDraftInstance,
    onInstanceModelsStatusChange,
  ]);

  // 5. Search + tag filter.
  const { search, tag, setSearch, setTag, filteredModels, allTags } =
    useModelsFilter(models);

  // 6. Per-model verify state + batch verify.
  const { verify, handleVerify, batchVerifying, handleBatchVerify } =
    useModelVerify({
      providerName,
      resolveCreds,
      instanceModels,
      instance,
      getFormValues,
      verifyTransform,
    });

  const handleBatchVerifyClick = useCallback(() => {
    handleBatchVerify(filteredModels);
  }, [filteredModels, handleBatchVerify]);

  // 7. Add / remove / batch toggle / custom add.
  const {
    allFilteredAdded,
    handleAddModel,
    handleRemoveModel,
    handleAddCustom,
    handleBatchToggleModels,
    batchLoading,
  } = useModelMutations({
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
    setDraftModelsList: applyDraftModelsList,
  });

  // 8. Edit dialog state + submit.
  const {
    editingModel,
    setEditingModel,
    editModelDialogFields,
    editDefaultValues,
    handleEditSubmit,
    editLoading,
    customModelDialogFields,
    providerFeatureKeys,
  } = useModelEdit({
    providerName,
    instanceName,
    addedSet,
    isDraftInstance,
    updateCatalogModel,
    clearCatalogOverride,
    updateDraftModel,
  });

  // Add-custom-model dialog open state (local UI state).
  const [dialogOpen, setDialogOpen] = useState(false);

  // Mirror dialog open state up to the host so it can pause its
  // blur-driven auto-save while the dialog is open (focus shifts into a
  // React Portal outside the host's onBlurCapture container).
  useEffect(() => {
    const open = dialogOpen || editingModel !== null;
    onBlurSuppressChange?.(open);
    return () => {
      if (open) onBlurSuppressChange?.(false);
    };
  }, [dialogOpen, editingModel, onBlurSuppressChange]);

  // hideIfEmpty: render nothing once the first fetch completes with no models.
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

      <div className="flex flex-col gap-2 border rounded-sm p-5 border-border-button">
        <div className="flex flex-col gap-2 ">
          <div className="flex items-center gap-2">
            <SearchInput
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t('setting.search')}
              rootClassName="flex-1"
            />
          </div>
          <div className="flex flex-wrap gap-1.5">
            <TagFilterButton
              label={tSetting('allModels')}
              count={models.length}
              active={tag === null}
              onClick={() => setTag(null)}
            />
            {allTags.map((tKey) => (
              <TagFilterButton
                key={tKey}
                label={mapModelKey[tKey as keyof typeof mapModelKey] || tKey}
                count={
                  models.filter((m) => m.model_types?.includes(tKey)).length
                }
                active={tag === tKey}
                onClick={() => setTag(tag === tKey ? null : tKey)}
              />
            ))}
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleBatchVerifyClick}
            disabled={batchVerifying || filteredModels.length === 0}
            data-testid="models-batch-verify"
            className="ml-auto"
          >
            {batchVerifying ? (
              <Loader2 className="size-3 animate-spin" />
            ) : (
              <ShieldCheck className="size-3" />
            )}
            {tSetting('batchVerifyModels')}
          </Button>
          {!hideActions && (
            // When the toggle is in "remove all" mode the click opens a
            // confirmation dialog instead of mutating directly; the button
            // acts as the dialog trigger, so the handler moves to `onOk`.
            <ConfirmDeleteDialog
              hidden={!allFilteredAdded}
              onOk={handleBatchToggleModels}
              title={t('common.removeModalTitle')}
              okButtonText={t('common.remove')}
            >
              <Button
                variant="outline"
                size="sm"
                onClick={allFilteredAdded ? undefined : handleBatchToggleModels}
                disabled={batchLoading || filteredModels.length === 0}
                data-testid="models-batch-toggle"
              >
                {batchLoading && <Loader2 className="size-3 animate-spin" />}
                {allFilteredAdded
                  ? tSetting('batchRemoveModels')
                  : tSetting('batchAddModels')}
              </Button>
            </ConfirmDeleteDialog>
          )}
        </div>

        <div className="bg-bg-card rounded-lg max-h-80 overflow-auto scrollbar-auto border border-border-button">
          {filteredModels.length === 0 ? (
            <div className="flex items-center justify-center text-text-secondary text-sm py-6 gap-2">
              <Search className="size-4" />
              {t('setting.listModelsEmpty')}
            </div>
          ) : (
            <ul>
              {filteredModels.map((model) => (
                <ModelRow
                  key={model.name}
                  model={model}
                  isAdded={addedSet.has(model.name)}
                  verifyStatus={verify[model.name] ?? 'idle'}
                  hideActions={hideActions}
                  onVerify={() => handleVerify(model)}
                  onAdd={() => handleAddModel(model)}
                  onRemove={() => handleRemoveModel(model)}
                  onEdit={() => setEditingModel(model)}
                  editLabel={tSetting('editModel')}
                />
              ))}
            </ul>
          )}
        </div>
      </div>

      <AddCustomModelDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={tSetting('addCustomModelTitle')}
        fields={customModelDialogFields}
        existingNames={models.map((m) => m.name)}
        providerFeatureKeys={providerFeatureKeys}
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
        existingNames={models
          .filter((m) => m.name !== editingModel?.name)
          .map((m) => m.name)}
        providerFeatureKeys={providerFeatureKeys}
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
