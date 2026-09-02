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

import { BuiltinPipelineItem } from '@/components/builtin-pipeline-form-field';
import { DataFlowSelect } from '@/components/data-pipeline-select';
import { ParseTypeItem } from '@/components/parse-type-form-field';
import PipelineOperatorTabs from '@/components/pipeline-operator-tabs';
import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Form } from '@/components/ui/form';
import { ParseType } from '@/constants/knowledge';
import { IModalProps } from '@/interfaces/common';
import { IChangeParserRequestBody } from '@/interfaces/request/document';
import { useCallback } from 'react';
import { FieldErrors, useFormState } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
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
  } = useDocumentPipelineForm({ parserId, pipelineId, parserConfig });

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
      // Surface the first failing operator tab so its field errors are visible.
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
          <DialogTitle>{t('knowledgeDetails.chunkMethod')}</DialogTitle>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit, onInvalid)}
            className="space-y-6 max-h-[70vh] overflow-auto -mx-6 px-10 py-5"
            id={FormId}
          >
            <ParseTypeItem />
            {parseType === ParseType.BuiltIn && <BuiltinPipelineItem />}
            {parseType === ParseType.Pipeline && (
              <DataFlowSelect
                isMult={false}
                showToDataPipeline={true}
                formFieldName="pipeline_id"
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
