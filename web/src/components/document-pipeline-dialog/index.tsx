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

import { ParserSelect } from '@/components/parser-select';
import PipelineOperatorTabs from '@/components/pipeline-operator-tabs';
import { PipelineVersionSelect } from '@/components/pipeline-version-select';
import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Form, FormField, FormItem, FormMessage } from '@/components/ui/form';
import { RAGFlowTooltip } from '@/components/ui/tooltip';
import { ParseType } from '@/constants/knowledge';
import { IModalProps } from '@/interfaces/common';
import { IChangeParserRequestBody } from '@/interfaces/request/document';
import { useCallback } from 'react';
import { FieldErrors, useFormState, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { buildParserSelectValue } from '@/hooks/use-parser-options';
import { useParserSelectHandler } from '@/hooks/use-parser-select-handler';
import {
  IDocumentPipelineDialogProps,
  useDocumentPipelineForm,
} from './use-document-pipeline-form';

const FormId = 'DocumentPipelineDialogForm';

interface IProps
  extends IModalProps<IChangeParserRequestBody>, IDocumentPipelineDialogProps {
  loading: boolean;
}

export function DocumentPipelineDialog({
  hideModal,
  onOk,
  parserId,
  pipelineId,
  pipelineVersionId,
  parserConfig,
  loading,
}: IProps) {
  const { t } = useTranslation();

  const {
    form,
    parseType,
    operatorNodes,
    operatorNodesLoading,
    activeTab,
    setActiveTab,
    handleOperatorValuesChange,
    operatorValues,
    showOperatorTabs,
    buildSubmitData,
    versions,
    latestVersionId,
    versionListLoading,
    versionListError,
  } = useDocumentPipelineForm({
    parserId,
    pipelineId,
    pipelineVersionId,
    parserConfig,
  });

  const handleParserSelect = useParserSelectHandler(form);

  const selectedParserId = useWatch({
    control: form.control,
    name: 'parser_id',
  });
  const selectedPipelineId = useWatch({
    control: form.control,
    name: 'pipeline_id',
  });
  const selectedVersionId = useWatch({
    control: form.control,
    name: 'pipeline_version_id',
  });

  // versions / latestVersionId / versionListLoading / versionListError come
  // from the form hook, which fetches the canvas version list reactively.
  // versions is newest-first, so versions[0] is the latest.
  // A selected version is only Missing when the list has finished loading and
  // the id is genuinely absent. While loading (initial mount of a pinned doc,
  // or after switching pipelines) the list is empty, so treating absence as
  // Missing would flash the banner and disable save before the fetch resolves.
  // A fetch error must NOT be treated as Missing either: an errored list is an
  // unknown state, not proof the version is gone, so we neither warn nor block
  // the save on it.
  const versionMissing =
    !!selectedVersionId &&
    !versionListLoading &&
    !versionListError &&
    !versions.some((v) => v.id === selectedVersionId);
  const versionOutdated =
    !!selectedVersionId &&
    !versionMissing &&
    selectedVersionId !== latestVersionId;

  const onSubmit = useCallback(
    async (data: Parameters<typeof buildSubmitData>[0]) => {
      const ret = await onOk?.(buildSubmitData(data));
      if (ret) {
        hideModal?.();
      }
    },
    [buildSubmitData, hideModal, onOk],
  );

  const onInvalid = useCallback(
    (errors: FieldErrors) => {
      const firstOperatorId = Object.keys(errors?.parser_config ?? {})[0];
      if (firstOperatorId) {
        setActiveTab(firstOperatorId);
      }
    },
    [setActiveTab],
  );

  const { errors } = useFormState({
    control: form.control,
    name: 'parser_config',
  });

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="max-w-[50vw] text-text-primary">
        <DialogHeader>
          <DialogTitle>{t('knowledgeDetails.parsingMode')}</DialogTitle>
        </DialogHeader>

        {versionMissing && (
          <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {t('flow.version.missingContent')}
          </div>
        )}

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit, onInvalid)}
            className="space-y-6 max-h-[70vh] overflow-auto -mx-6 px-10 py-5"
            id={FormId}
          >
            <FormField
              control={form.control}
              name={
                parseType === ParseType.BuiltIn ? 'parser_id' : 'pipeline_id'
              }
              render={() => (
                <FormItem>
                  <ParserSelect
                    value={buildParserSelectValue({
                      parseType,
                      parserId: selectedParserId,
                      pipelineId: selectedPipelineId,
                    })}
                    onChange={handleParserSelect}
                  />
                  <FormMessage />
                </FormItem>
              )}
            />
            {parseType === ParseType.Pipeline && !!selectedPipelineId && (
              <FormField
                control={form.control}
                name="pipeline_version_id"
                render={({ field }) => (
                  <FormItem>
                    <div className="flex items-center gap-1.5">
                      <span className="text-sm text-text-secondary">
                        {t('flow.version.label')}
                      </span>
                      {versionOutdated && (
                        <RAGFlowTooltip tooltip={t('flow.version.outdated')}>
                          <span className="text-destructive cursor-help font-bold">
                            !
                          </span>
                        </RAGFlowTooltip>
                      )}
                    </div>
                    <PipelineVersionSelect
                      versions={versions}
                      value={field.value ?? ''}
                      onChange={field.onChange}
                    />
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            {showOperatorTabs && (
              <PipelineOperatorTabs
                nodes={operatorNodes}
                activeTab={activeTab}
                onTabChange={setActiveTab}
                onOperatorValuesChange={handleOperatorValuesChange}
                operatorValues={operatorValues}
                operatorFormErrors={
                  errors.parser_config as
                    | Record<string, FieldErrors | undefined>
                    | undefined
                }
                fixedFileFormats
              />
            )}
          </form>
        </Form>
        <DialogFooter>
          <ButtonLoading
            type="submit"
            form={FormId}
            disabled={versionMissing}
            loading={
              loading || (operatorNodesLoading && operatorNodes.length === 0)
            }
          >
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
