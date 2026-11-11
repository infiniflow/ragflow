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

import LLMLabel, {
  MissingModelLabel,
  PlatformModelBadge,
} from '@/components/llm-select/llm-label';
import { LlmIcon } from '@/components/svg-icon';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { ModelTypeProviderMap } from '@/constants/llm';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { IAddedModel } from '@/interfaces/database/llm';
import {
  buildModelValue,
  getRealModelName,
  parseModelValue,
} from '@/utils/llm-util';
import { forwardRef, useCallback, useEffect, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { TreeSelect, TreeSelectNode } from './tree-select';

/** Maps form field names to their supported model types */
export const ModelTypeMap: Record<string, string[]> = {
  llm_id: ['chat', 'vision'],
  decision_id: ['decision'],
  embd_id: ['embedding'],
  img2txt_id: ['vision'],
  asr_id: ['asr'],
  rerank_id: ['rerank'],
  tts_id: ['tts'],
};

/** Lets "platform" in the search box find shared platform providers. */
const PlatformSearchKeyword = 'platform';

/**
 * The predicate shared by buildModelTree and useModelTree. A disabled
 * platform model cannot be enabled by the tenant, so it is not offered; a
 * saved reference to it shows as unavailable.
 */
function filterAddedModels(
  allModels: IAddedModel[],
  modelTypes: string[],
  providers?: string[],
): IAddedModel[] {
  return allModels.filter(
    (m) =>
      m.model_type?.some((t) => modelTypes.includes(t)) &&
      (!providers?.length || providers.includes(m.provider_name)) &&
      (m.source !== 'platform' ||
        (m.status !== 'inactive' && m.instance_status !== 'inactive')),
  );
}

export function buildModelTree(
  allModels: IAddedModel[],
  modelTypes: string[],
  renderLeafLabel?: (
    node: TreeSelectNode,
    model: IAddedModel,
  ) => React.ReactNode,
  providers?: string[],
  platformLabel?: string,
): TreeSelectNode[] {
  const filtered = filterAddedModels(allModels, modelTypes, providers);

  const seenLeafIds = new Set<string>();
  // Platform models are grouped apart from the tenant's own provider of the
  // same name, so identical names stay distinguishable by source.
  const providerMap = new Map<string, Map<string, IAddedModel[]>>();
  const groupKey = (model: IAddedModel) =>
    model.source === 'platform'
      ? `platform:${model.provider_name}`
      : model.provider_name;

  for (const model of filtered) {
    let instances = providerMap.get(groupKey(model));
    if (!instances) {
      instances = new Map();
      providerMap.set(groupKey(model), instances);
    }
    let modelList = instances.get(model.instance_name);
    if (!modelList) {
      modelList = [];
      instances.set(model.instance_name, modelList);
    }
    modelList.push(model);
  }

  return Array.from(providerMap.entries()).map(([key, instances]) => {
    const first = instances.values().next().value?.[0];
    const provider = first?.provider_name ?? key;
    const platform = first?.source === 'platform';
    return {
      id: key,
      title: provider,
      label: platform ? (
        <span className="flex items-center gap-1.5 min-w-0">
          <span className="truncate">{provider}</span>
          <PlatformModelBadge />
        </span>
      ) : undefined,
      searchKeywords: platform
        ? [PlatformSearchKeyword, ...(platformLabel ? [platformLabel] : [])]
        : undefined,
      children: buildInstanceNodes(key, instances),
    };
  });

  function buildInstanceNodes(
    key: string,
    instances: Map<string, IAddedModel[]>,
  ): TreeSelectNode[] {
    return Array.from(instances.entries()).map(([instance, models]) => ({
      id: `${key}||${instance}`,
      title: instance,
      children: models.reduce<TreeSelectNode[]>((acc, m) => {
        const modelName = getRealModelName(m.name);

        const id =
          m.model_id ||
          buildModelValue({
            model_name: modelName,
            model_instance: m.instance_name,
            model_provider: m.provider_name,
          });
        if (seenLeafIds.has(id)) return acc;
        seenLeafIds.add(id);
        const leafNode: TreeSelectNode = {
          id,
          title: modelName,
          label: (
            <span className="flex items-center gap-1.5 truncate">
              <LlmIcon
                name={m.provider_name}
                width={22}
                height={22}
                imgClass="size-[22px] flex-shrink-0"
              />
              <span className="truncate">{modelName}</span>
            </span>
          ),
          data: {
            provider_name: m.provider_name,
            instance_name: m.instance_name,
            model_name: modelName,
            source: m.source,
          },
        };
        if (renderLeafLabel) {
          leafNode.label = renderLeafLabel(leafNode, m);
        }
        acc.push(leafNode);
        return acc;
      }, []),
    }));
  }
}

/**
 * Selected-value display for pickers that render a model tree directly:
 * shows the leaf label and marks a platform model, so a platform model and a
 * same-named tenant model look different once selected.
 */
export function renderSelectedModelNode(node: TreeSelectNode | undefined) {
  if (!node) return null;
  const content = node.label ?? node.title;
  // Options that are not models (DeepDOC, plain text) show as listed.
  if (!node.data?.model_name) return content;
  const fullName = [node.data.model_name, node.data.instance_name]
    .filter(Boolean)
    .join(' ');
  return (
    <span className="flex items-center gap-1.5 min-w-0">
      <span className="min-w-0 truncate" title={fullName}>
        {content}
      </span>
      {node.data.source === 'platform' && <PlatformModelBadge />}
    </span>
  );
}

export interface UseModelTreeOptions {
  modelTypes?: string[];
  /**
   * Effective provider restriction. The ModelTypeProviderMap fallback stays
   * with ModelTreeSelect, so callers that pass nothing get every provider.
   */
  providers?: string[];
  renderLeafLabel?: (
    node: TreeSelectNode,
    model: IAddedModel,
  ) => React.ReactNode;
  ownerTenantId?: string;
}

/**
 * Fetch + filter + tree assembly shared by ModelTreeSelect and the pickers
 * that need the query result itself: layout_recognize derives the selected
 * provider from allAddedModels, and the image OCR radio drives its state
 * machine from the flat list and the request state. Array options must be
 * stable references (module constants) — the memoized tree keys on their
 * identity, and a fresh array per render would reset TreeSelect expansion on
 * every parent render.
 */
export function useModelTree({
  modelTypes = ModelTypeMap.llm_id,
  providers,
  renderLeafLabel,
  ownerTenantId,
}: UseModelTreeOptions = {}) {
  const { t } = useTranslation();
  const platformLabel = t('setting.platformProvided');
  const {
    data: allAddedModels,
    isFetched,
    isError,
  } = useFetchAllAddedModels(undefined, ownerTenantId);

  const models = useMemo(
    () => filterAddedModels(allAddedModels, modelTypes, providers),
    [allAddedModels, modelTypes, providers],
  );

  const treeData = useMemo(
    () =>
      buildModelTree(
        allAddedModels,
        modelTypes,
        renderLeafLabel,
        providers,
        platformLabel,
      ),
    [allAddedModels, modelTypes, renderLeafLabel, providers, platformLabel],
  );

  return { treeData, models, allAddedModels, isFetched, isError };
}

export interface ModelTreeSelectProps {
  modelTypes?: string[];
  /**
   * Restricts the tree to these providers. Defaults to whatever
   * ModelTypeProviderMap declares for the selected types, so a caller that
   * picks modelTypes automatically inherits the provider restriction.
   */
  providers?: string[];
  value?: string;
  onChange?: (value: string) => void;
  disabled?: boolean;
  placeholder?: string;
  showSearch?: boolean;
  allowClear?: boolean;
  className?: string;
  renderSelected?: (node: TreeSelectNode | undefined) => React.ReactNode;
  testId?: string;
  ownerTenantId?: string;
  /** Name shown for a saved value that matches no available model. */
  missingValueLabel?: string;
}

export const ModelTreeSelect = forwardRef<
  HTMLButtonElement,
  ModelTreeSelectProps
>(function ModelTreeSelect(
  {
    modelTypes = ModelTypeMap.llm_id,
    providers,
    value,
    onChange,
    disabled,
    placeholder,
    showSearch = true,
    allowClear = false,
    className,
    renderSelected,
    testId,
    ownerTenantId,
    missingValueLabel,
  },
  ref,
) {
  const { t } = useTranslation();
  // An explicit prop wins; otherwise inherit the restriction declared for the
  // type, so the default-model page and the chat settings agree without each
  // call site having to remember.
  const allowedProviders = useMemo(() => {
    if (providers) return providers;
    for (const type of modelTypes) {
      const declared = ModelTypeProviderMap[type];
      if (declared) return declared;
    }
    return undefined;
  }, [providers, modelTypes]);

  const {
    treeData,
    isFetched: modelsFetched,
    isError: modelsError,
  } = useModelTree({
    modelTypes,
    providers: allowedProviders,
    ownerTenantId,
  });

  // Backward compatibility: map legacy concatenated ids
  // ("modelName@instanceName@providerName") to new model_id-based ids so
  // that previously persisted values still display correctly.
  const legacyIdMap = useMemo(() => {
    const map = new Map<string, string>();
    const walk = (nodes: TreeSelectNode[]) => {
      for (const node of nodes) {
        if (node.children?.length) {
          walk(node.children);
        } else if (node.data && node.data.source !== 'platform') {
          // Saved composite names refer to the tenant's own models only.
          const legacyId = buildModelValue({
            model_name: node.data.model_name,
            model_instance: node.data.instance_name,
            model_provider: node.data.provider_name,
          });
          map.set(legacyId, node.id);
        }
      }
    };
    walk(treeData);
    return map;
  }, [treeData]);

  const normalizedValue = useMemo(() => {
    if (!value) return value;
    // A two-part "model@provider" name refers to the default instance.
    const parsed = parseModelValue(value);
    const canonical = parsed ? buildModelValue(parsed) : value;
    return legacyIdMap.get(value) ?? legacyIdMap.get(canonical) ?? value;
  }, [value, legacyIdMap]);

  const defaultRenderSelected = useCallback(
    (node: TreeSelectNode | undefined) => {
      // Without a value, show the placeholder so the trigger is named. A
      // saved value whose model list is still loading shows nothing yet.
      if (!node?.data) {
        return value ? '' : (placeholder ?? t('common.pleaseSelect'));
      }
      return (
        <LLMLabel
          value={buildModelValue({
            model_name: node.data.model_name,
            model_instance: node.data.instance_name,
            model_provider: node.data.provider_name,
          })}
          platform={node.data.source === 'platform'}
        />
      );
    },
    [placeholder, value, t],
  );

  // The persisted model no longer matches any added model (e.g. it was
  // deleted from the provider) — keep it visible with a warning marker
  // instead of rendering a blank select. Prefer the readable model name over
  // the raw composite id.
  const renderMissingModel = useCallback(
    (missingValue: string) => (
      <MissingModelLabel
        value={missingValue}
        ownerTenantId={ownerTenantId}
        label={missingValueLabel}
      />
    ),
    [ownerTenantId, missingValueLabel],
  );

  return (
    <TreeSelect
      ref={ref}
      data={treeData}
      value={normalizedValue}
      onChange={onChange}
      placeholder={placeholder}
      disabled={disabled}
      showSearch={showSearch}
      allowClear={allowClear}
      defaultExpandAll
      className={className}
      renderSelected={renderSelected ?? defaultRenderSelected}
      renderMissingValue={renderMissingModel}
      loading={!modelsFetched || modelsError}
      testId={testId}
    />
  );
});

export interface ModelTreeSelectFormFieldProps extends ModelTreeSelectProps {
  name?: string;
  label?: string;
  tooltip?: string;
  required?: boolean;
}

export function ModelTreeSelectFormField({
  name = 'llm_id',
  label,
  tooltip,
  required,
  ...rest
}: ModelTreeSelectFormFieldProps) {
  const form = useFormContext();
  const { t } = useTranslation();
  const { loading } = useFetchAllAddedModels(undefined, rest.ownerTenantId);
  const value = useWatch({ control: form.control, name });

  // `form` from context is a new object on every provider render, so it must
  // not be an effect dependency — `trigger` is a stable control method. With
  // `form` in the deps, each trigger() emits formState updates that re-render
  // the provider, which recreates `form`, which refires this effect: an
  // infinite validation loop whenever the field keeps failing validation.
  const trigger = form.trigger;

  // A persisted value never fires onChange validation, so once the model list
  // has loaded, revalidate explicitly — it may reference a model that has
  // since been deleted, and the error should be visible before submit.
  useEffect(() => {
    if (loading || !value) return;
    trigger(name);
  }, [trigger, loading, name, value]);

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          {label && (
            <FormLabel required={required} tooltip={tooltip}>
              {label}
            </FormLabel>
          )}
          <FormControl>
            <ModelTreeSelect
              {...rest}
              ref={field.ref}
              value={field.value}
              onChange={field.onChange}
              placeholder={rest.placeholder ?? t('common.pleaseSelect')}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
