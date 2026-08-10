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
import { Checkbox } from '@/components/ui/checkbox';
import { SearchInput } from '@/components/ui/input';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import { useFetchInstanceModels } from '@/hooks/use-llm-request';
import { IProviderModelItem } from '@/interfaces/request/llm';
import {
  ListMinus,
  ListPlus,
  Loader2,
  Plus,
  Search,
  ShieldCheck,
} from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AddCustomModelDialog } from '../add-custom-model-dialog';
import { mapModelKey } from '../available-models';
import { ModelRow } from './components/model-row';
import { TagFilterButton } from './components/tag-filter-button';
import {
  DRAFT_INSTANCE_SENTINEL,
  hasKnownModelTypes,
  normalizeModelTypes,
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
    deferModelMutations = false,
    hideIfEmpty = false,
    getFormValues,
    verifyTransform,
    buildInstanceUpdatePayload,
    instanceDetailsLoaded,
    onBlurSuppressChange,
    onInstanceModelsChange,
    onInstanceModelsEdited,
  } = props;

  const isDraftInstance =
    !instanceName || instanceName === DRAFT_INSTANCE_SENTINEL;
  const useLocalModels = isDraftInstance || deferModelMutations;

  // 1. Credentials for catalog / verify / batch calls.
  const { resolveCreds } = useResolveCreds(instance, getFormValues);

  // Snapshot of the current api_key so `useModelsCatalog` can gate the
  // auto-fetch for VolcEngine on the user actually having typed one.
  // Recomputed on every render so the effect re-runs as soon as the
  // form value lands.
  const currentCreds = resolveCreds();

  // 2. Per-instance saved models (shared by catalog, derived, verify).
  const { data: instanceModels } = useFetchInstanceModels(
    providerName,
    instanceName,
  );

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
    regionValue: currentCreds.region,
    authMode: currentCreds.extensions.auth_mode,
  });

  // 3a. Locally tracked model selection. New instances start empty and
  // auto-populate from the catalog. Saved instances whose credentials
  // are being edited start from their persisted selection and defer all
  // mutations until the host saves the credential and model changes.
  const [draftModels, setDraftModels] = useState<IProviderModelItem[]>([]);
  // Tracks whether we've auto-populated the draft from the catalog for
  // the current draft session. Prevents re-adding models the user has
  // manually removed when the catalog refetches.
  const hasAutoPopulatedDraftRef = useRef(false);
  const hasSeededDeferredModelsRef = useRef(false);
  useEffect(() => {
    setDraftModels([]);
    hasAutoPopulatedDraftRef.current = false;
    hasSeededDeferredModelsRef.current = false;
  }, [providerName, instanceName, deferModelMutations]);

  useEffect(() => {
    if (!deferModelMutations || hasSeededDeferredModelsRef.current) return;
    if (!instanceModels) return;
    hasSeededDeferredModelsRef.current = true;
    setDraftModels(
      instanceModels.map((model) => ({
        name: model.name,
        max_tokens: model.max_tokens ?? 0,
        model_types: normalizeModelTypes(model.model_type),
        features: model.is_tools ? ['is_tools'] : [],
        extra: model.extra,
      })),
    );
  }, [deferModelMutations, instanceModels]);

  // Auto-populate models whose capabilities are known. Availability-only
  // catalog candidates stay visible but require explicit configuration.
  // The flag prevents re-adding removed models after a catalog refetch;
  // pre-existing manual additions are preserved by the merge below.
  useEffect(() => {
    if (!isDraftInstance) return;
    if (hasAutoPopulatedDraftRef.current) return;
    if (catalog.length === 0) return;
    hasAutoPopulatedDraftRef.current = true;
    setDraftModels((prev) => {
      const existing = new Set(prev.map((m) => m.name));
      return [
        ...prev,
        ...catalog.filter(
          (model) => hasKnownModelTypes(model) && !existing.has(model.name),
        ),
      ];
    });
  }, [isDraftInstance, catalog]);

  const addDraftModel = useCallback((model: IProviderModelItem) => {
    setDraftModels((prev) =>
      prev.some((m) => m.name === model.name) ? prev : [...prev, model],
    );
  }, []);
  const removeDraftModel = useCallback((name: string) => {
    setDraftModels((prev) => prev.filter((m) => m.name !== name));
  }, []);
  const updateDraftModel = useCallback((item: IProviderModelItem) => {
    setDraftModels((prev) =>
      prev.map((m) => (m.name === item.name ? { ...m, ...item } : m)),
    );
  }, []);

  // 4. Derived union list (instance ∪ catalog) + push to host.
  const { instanceItems, models, addedSet } = useModelsDerived({
    catalog,
    instanceModels,
    draftModels,
    isDraftInstance: useLocalModels,
    onInstanceModelsChange,
  });

  // 5. Search + tag filter.
  const { search, tag, setSearch, setTag, filteredModels, allTags } =
    useModelsFilter(models);

  // 6. Per-model verify state + batch verify.
  const { verify, handleVerify, batchVerifying, handleBatchVerify } =
    useModelVerify({
      providerName,
      resolveCreds,
      instanceModels,
      instance: useLocalModels ? undefined : instance,
      getFormValues,
      verifyTransform,
    });

  // 6a. Model selection for batch verify.
  const [selectedModels, setSelectedModels] = useState<Set<string>>(new Set());

  const verifiableModels = useMemo(
    () => filteredModels.filter(hasKnownModelTypes),
    [filteredModels],
  );

  const toggleModel = useCallback((name: string) => {
    setSelectedModels((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
  }, []);

  const toggleAllFiltered = useCallback(() => {
    setSelectedModels((prev) => {
      const allSelected = verifiableModels.every((m) => prev.has(m.name));
      const next = new Set(prev);
      if (allSelected) {
        verifiableModels.forEach((m) => next.delete(m.name));
      } else {
        verifiableModels.forEach((m) => next.add(m.name));
      }
      return next;
    });
  }, [verifiableModels]);

  const selectedVerifiableModels = useMemo(
    () => verifiableModels.filter((m) => selectedModels.has(m.name)),
    [verifiableModels, selectedModels],
  );

  const selectAllChecked: boolean | 'indeterminate' =
    selectedVerifiableModels.length === 0
      ? false
      : selectedVerifiableModels.length === verifiableModels.length
        ? true
        : 'indeterminate';

  const handleBatchVerifyClick = useCallback(() => {
    handleBatchVerify(selectedVerifiableModels);
  }, [selectedVerifiableModels, handleBatchVerify]);

  // 7. Add / remove / batch toggle / custom add.
  const {
    allFilteredAdded,
    canBatchToggle,
    handleAddModel,
    handleRemoveModel,
    handleAddCustom,
    handleBatchToggleModels,
    addLoading,
    batchLoading,
  } = useModelMutations({
    providerName,
    instanceName,
    isDraftInstance: useLocalModels,
    hideActions,
    instanceItems,
    filteredModels,
    addedSet,
    setCatalog,
    clearCatalogOverride,
    addDraftModel,
    removeDraftModel,
    setDraftModelsList: setDraftModels,
    buildInstanceUpdatePayload,
    onInstanceModelsEdited,
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
    isDraftInstance: useLocalModels,
    updateCatalogModel,
    clearCatalogOverride,
    updateDraftModel,
    onInstanceModelsEdited,
  });

  const [candidateAddName, setCandidateAddName] = useState<string | null>(null);
  const [candidateAddLoading, setCandidateAddLoading] = useState(false);
  const editDialogLoading = editLoading || candidateAddLoading;

  const createAddHandler = (model: IProviderModelItem) => () => {
    if (hasKnownModelTypes(model)) {
      return handleAddModel(model);
    }
    setCandidateAddName(model.name);
    setEditingModel(model);
  };

  const createEditHandler = (model: IProviderModelItem) => () => {
    setCandidateAddName(null);
    setEditingModel(model);
  };

  const handleEditDialogOpenChange = (open: boolean) => {
    if (open || editDialogLoading) return;
    setCandidateAddName(null);
    setEditingModel(null);
  };

  const handleEditDialogSubmit = async (item: IProviderModelItem) => {
    if (candidateAddName !== item.name) {
      await handleEditSubmit(item);
      return;
    }
    if (!hasKnownModelTypes(item)) return;
    if (candidateAddLoading) return;
    setCandidateAddLoading(true);
    try {
      for (const modelType of item.model_types) {
        if (!(await handleVerify({ ...item, model_types: [modelType] }))) {
          return;
        }
      }
      if (!(await handleAddModel(item))) return;
      setCandidateAddName(null);
      setEditingModel(null);
    } finally {
      setCandidateAddLoading(false);
    }
  };

  // Add-custom-model dialog open state (local UI state).
  const [dialogOpen, setDialogOpen] = useState(false);
  const [customAddLoading, setCustomAddLoading] = useState(false);
  const customDialogLoading = addLoading || customAddLoading;

  const handleCustomDialogOpenChange = (open: boolean) => {
    if (!open && customDialogLoading) return;
    setDialogOpen(open);
  };

  const handleCustomDialogSubmit = async (item: IProviderModelItem) => {
    if (customAddLoading) return;
    setCustomAddLoading(true);
    try {
      if (await handleAddCustom(item)) setDialogOpen(false);
    } finally {
      setCustomAddLoading(false);
    }
  };

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
            {!hideActions && (
              <Button
                variant="outline"
                size="icon-sm"
                onClick={handleBatchToggleModels}
                disabled={batchLoading || !canBatchToggle}
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
          <Checkbox
            checked={selectAllChecked}
            onCheckedChange={toggleAllFiltered}
            disabled={batchVerifying || verifiableModels.length === 0}
            aria-label={tSetting('selectAllFiltered')}
          />
          <span className="text-sm text-text-secondary">
            {tSetting('selectAllFiltered')}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={handleBatchVerifyClick}
            disabled={selectedVerifiableModels.length === 0 || batchVerifying}
            data-testid="models-batch-verify"
            className="ml-auto"
          >
            {batchVerifying ? (
              <Loader2 className="size-3 animate-spin" />
            ) : (
              <ShieldCheck className="size-3" />
            )}
            {tSetting('batchVerifySelected', {
              count: selectedVerifiableModels.length,
            })}
          </Button>
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
                  isSelected={
                    hasKnownModelTypes(model) && selectedModels.has(model.name)
                  }
                  onToggleSelect={
                    hasKnownModelTypes(model)
                      ? () => toggleModel(model.name)
                      : undefined
                  }
                  onVerify={
                    hasKnownModelTypes(model)
                      ? () => handleVerify(model)
                      : undefined
                  }
                  onAdd={createAddHandler(model)}
                  onRemove={() => handleRemoveModel(model)}
                  onEdit={
                    addedSet.has(model.name) || hasKnownModelTypes(model)
                      ? createEditHandler(model)
                      : undefined
                  }
                  editLabel={tSetting('editModel')}
                />
              ))}
            </ul>
          )}
        </div>
      </div>

      <AddCustomModelDialog
        open={dialogOpen}
        onOpenChange={handleCustomDialogOpenChange}
        title={tSetting('addCustomModelTitle')}
        fields={customModelDialogFields}
        existingNames={models.map((m) => m.name)}
        providerFeatureKeys={providerFeatureKeys}
        loading={customDialogLoading}
        onSubmit={handleCustomDialogSubmit}
        submitText={tc('confirm')}
        cancelText={tc('cancel')}
      />

      <AddCustomModelDialog
        open={editingModel !== null}
        onOpenChange={handleEditDialogOpenChange}
        title={tSetting('editModel')}
        fields={editModelDialogFields}
        existingNames={models
          .filter((m) => m.name !== editingModel?.name)
          .map((m) => m.name)}
        providerFeatureKeys={providerFeatureKeys}
        defaultValues={editDefaultValues}
        loading={editDialogLoading}
        onSubmit={handleEditDialogSubmit}
        submitText={tc('confirm')}
        cancelText={tc('cancel')}
      />
    </div>
  );
}

export default ModelsSection;
