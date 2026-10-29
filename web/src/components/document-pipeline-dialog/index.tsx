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

import { ParserTreeSelect } from '@/components/parser-tree-select';
import PipelineOperatorTabs from '@/components/pipeline-operator-tabs';
import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Form, FormField, FormItem, FormMessage } from '@/components/ui/form';
import { ParseType } from '@/constants/knowledge';
import { IModalProps } from '@/interfaces/common';
import { IChangeParserRequestBody } from '@/interfaces/request/document';
import { useCallback } from 'react';
import { FieldErrors, useFormState, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
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
    parserSelectValue,
    operatorNodes,
    operatorNodesLoading,
    activeTab,
    setActiveTab,
    handleOperatorValuesChange,
    operatorValues,
    showOperatorTabs,
    buildSubmitData,
    latestVersionId,
    parserOptionMissing,
  } = useDocumentPipelineForm({
    parserId,
    pipelineId,
    pipelineVersionId,
    parserConfig,
  });

  const handleParserSelect = useParserSelectHandler(form);

  const selectedVersionId = useWatch({
    control: form.control,
    name: 'pipeline_version_id',
  });

  // Outdated = pinned to a non-latest version. A pinned version that was
  // deleted entirely is covered by the select's missing display plus the
  // parserOptionMissing guard below, so no separate Missing banner lives here.
  const versionOutdated =
    !!selectedVersionId && selectedVersionId !== latestVersionId;

  const onSubmit = useCallback(
    async (data: Parameters<typeof buildSubmitData>[0]) => {
      // A deleted pinned version/parser never reaches the backend; the
      // missing message beside the select is already visible.
      if (parserOptionMissing) return;
      const ret = await onOk?.(buildSubmitData(data));
      if (ret) {
        hideModal?.();
      }
    },
    [buildSubmitData, hideModal, onOk, parserOptionMissing],
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
              render={({ fieldState }) => (
                <FormItem>
                  <ParserTreeSelect
                    value={parserSelectValue}
                    onChange={handleParserSelect}
                    outdated={versionOutdated}
                    invalid={fieldState.invalid || parserOptionMissing}
                  />
                  <FormMessage />
                  {parserOptionMissing && (
                    <p className="text-sm font-medium text-state-error">
                      {t(
                        'knowledgeConfiguration.parserOptionUnavailableMessage',
                      )}
                    </p>
                  )}
                </FormItem>
              )}
            />
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
