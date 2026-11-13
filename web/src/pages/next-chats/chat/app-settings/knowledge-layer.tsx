'use client';

import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { FormLabel } from '@/components/ui/form';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { CompilationTemplateKind } from '@/constants/compilation';
import {
  compilationModelStatusLabel,
  useFetchCompilationModelVersions,
  useFetchCompilationModels,
} from '@/hooks/use-compilation-model-request';
import { IOntologyBinding } from '@/interfaces/database/chat';
import { prefixName } from '@/utils/form';
import { useCallback, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  OntologyOverrideMode,
  OntologyOverrideModeValue,
  OntologyVersionFollowPublished,
  ontologyBindingPayload,
  ontologyOverrideMode,
} from './ontology-binding';
import { OntologyPreview } from './ontology-preview';

interface KnowledgeLayerProps {
  prefix?: string;
}

/**
 * Which knowledge layer this assistant reads, and how it relates to the
 * deployment's default.
 *
 * The conversation's datasets and this scope answer different questions — "how
 * is this worded in the documents" against "what does the model say exists" —
 * which is why the scope is its own picker rather than the datasets above.
 *
 * The three-way mode is not decoration: "inherit" leaves the template's
 * declaration in force, and "off" is the only way to refuse it for one
 * assistant. A two-state switch could express the override but never its
 * removal, which is exactly the state a deployment default needs to be
 * escapable from.
 */
export function KnowledgeLayer({ prefix = '' }: KnowledgeLayerProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const bindingPath = prefixName(prefix, 'prompt_config.ontology');
  const binding = useWatch({
    control: form.control,
    name: bindingPath,
  }) as IOntologyBinding | null | undefined;
  const mode = ontologyOverrideMode(binding);

  const { models, loading: modelsLoading } = useFetchCompilationModels(
    CompilationTemplateKind.Ontology,
  );
  const { versions } = useFetchCompilationModelVersions(binding?.model_id);

  const modelOptions = useMemo(
    () =>
      models.map((model) => ({
        label: `${model.name} · ${compilationModelStatusLabel(model.publish_status)}`,
        value: model.id,
        description: model.version
          ? t('chat.knowledgeLayerModelPublishedVersion', {
              version: model.version,
            })
          : t('chat.knowledgeLayerModelNeverPublished'),
      })),
    [models, t],
  );

  const versionOptions = useMemo(
    () => [
      {
        value: String(OntologyVersionFollowPublished),
        label: t('chat.knowledgeLayerVersionFollowPublished'),
      },
      ...versions.map((version) => ({
        value: String(version.version),
        label: version.is_current
          ? t('chat.knowledgeLayerVersionCurrent', { version: version.version })
          : `v${version.version}`,
      })),
    ],
    [t, versions],
  );

  const writeBinding = useCallback(
    (next: IOntologyBinding | null) => {
      form.setValue(bindingPath, next, {
        shouldDirty: true,
        shouldValidate: true,
      });
    },
    [bindingPath, form],
  );

  const handleModeChange = useCallback(
    (next: string) => {
      writeBinding(
        ontologyBindingPayload(next as OntologyOverrideModeValue, {
          modelId: binding?.model_id,
          modelVersion: binding?.model_version,
          knowledgeDatasetIds: binding?.knowledge_dataset_ids,
        }),
      );
    },
    [binding, writeBinding],
  );

  const handleModelChange = useCallback(
    (modelId: string) => {
      // The pinned version is dropped on purpose: version numbers belong to one
      // model, and carrying one across would search a snapshot of the structure
      // that is no longer selected.
      writeBinding({
        ...(binding ?? {}),
        enabled: true,
        model_id: modelId,
        model_version: OntologyVersionFollowPublished,
      });
    },
    [binding, writeBinding],
  );

  const handleVersionChange = useCallback(
    (value: string) => {
      writeBinding({
        ...(binding ?? {}),
        enabled: true,
        model_version: Number(value),
      });
    },
    [binding, writeBinding],
  );

  return (
    <div className="space-y-4 rounded-lg border border-border-button p-3">
      <div className="space-y-1">
        <p className="text-sm text-text-primary">{t('chat.knowledgeLayer')}</p>
        <p className="text-xs text-text-secondary">
          {t('chat.knowledgeLayerTip')}
        </p>
      </div>

      <div className="space-y-2">
        <FormLabel>{t('chat.knowledgeLayerMode')}</FormLabel>
        <Select value={mode} onValueChange={handleModeChange}>
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={OntologyOverrideMode.Inherit}>
              {t('chat.knowledgeLayerModeInherit')}
            </SelectItem>
            <SelectItem value={OntologyOverrideMode.On}>
              {t('chat.knowledgeLayerModeOn')}
            </SelectItem>
            <SelectItem value={OntologyOverrideMode.Off}>
              {t('chat.knowledgeLayerModeOff')}
            </SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-text-secondary">
          {t('chat.knowledgeLayerModeHint')}
        </p>
      </div>

      {mode === OntologyOverrideMode.On && (
        <>
          <RAGFlowFormItem
            name={`${bindingPath}.model_id`}
            label={t('chat.knowledgeLayerModel')}
            required
          >
            {() => (
              <SelectWithSearch
                value={binding?.model_id}
                onChange={handleModelChange}
                options={modelOptions}
                loading={modelsLoading}
                placeholder={
                  modelOptions.length
                    ? t('chat.knowledgeLayerModelPlaceholder')
                    : t('chat.knowledgeLayerModelEmpty')
                }
              />
            )}
          </RAGFlowFormItem>

          <RAGFlowFormItem
            name={`${bindingPath}.model_version`}
            label={t('chat.knowledgeLayerVersion')}
            tooltip={t('chat.knowledgeLayerVersionTip')}
          >
            {() => (
              <Select
                value={String(
                  binding?.model_version ?? OntologyVersionFollowPublished,
                )}
                onValueChange={handleVersionChange}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {versionOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </RAGFlowFormItem>

          <KnowledgeBaseFormField
            name={`${bindingPath}.knowledge_dataset_ids`}
            label={t('chat.knowledgeLayerKnowledgeScope')}
            tooltip={t('chat.knowledgeLayerKnowledgeScopeTip')}
            allowEmptyDatasets
          />

          <OntologyPreview
            modelId={binding?.model_id}
            modelVersion={binding?.model_version}
            knowledgeDatasetIds={binding?.knowledge_dataset_ids}
          />
        </>
      )}
    </div>
  );
}
