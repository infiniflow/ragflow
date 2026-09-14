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

import { ParseType } from '@/constants/knowledge';
import {
  useActiveTab,
  usePipelineOperatorNodes,
  useResetParserConfigOnPipelineChange,
} from '@/hooks/use-pipeline-operator';
import { IChangeParserRequestBody } from '@/interfaces/request/document';
import {
  getOperatorType,
  transformFormConfigToApi,
  transformSavedParserConfigToForm,
} from '@/utils/pipeline-operator';
import { zodResolver } from '@hookform/resolvers/zod';
import { isEqual } from 'lodash';
import { useCallback, useEffect, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { addParserConfigIssues } from '../pipeline-operator-tabs/parser-config-validation';

export interface IDocumentPipelineDialogProps {
  parserId: string;
  pipelineId?: string;
  parserConfig?: Record<string, any>;
}

export function useDocumentPipelineForm({
  parserId,
  pipelineId,
  parserConfig,
}: IDocumentPipelineDialogProps) {
  const { t } = useTranslation();

  const FormSchema = useMemo(
    () =>
      z
        .object({
          parseType: z.nativeEnum(ParseType),
          parser_id: z.string(),
          pipeline_id: z.string().optional(),
          parser_config: z.record(z.string(), z.any()).optional(),
        })
        .superRefine((data, ctx) => {
          if (data.parseType === ParseType.BuiltIn && !data.parser_id.trim()) {
            ctx.addIssue({
              path: ['parser_id'],
              message: t('common.pleaseSelect'),
              code: 'custom',
            });
          }
          if (data.parseType === ParseType.Pipeline && !data.pipeline_id) {
            ctx.addIssue({
              path: ['pipeline_id'],
              message: t('common.pleaseSelect'),
              code: 'custom',
            });
          }
          addParserConfigIssues(data.parser_config, ctx, t);
        }),
    [t],
  );

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      parseType: pipelineId ? ParseType.Pipeline : ParseType.BuiltIn,
      parser_id: parserId || '',
      pipeline_id: pipelineId || '',
      parser_config: transformSavedParserConfigToForm(parserConfig),
    },
  });

  const parseType = useWatch({
    control: form.control,
    name: 'parseType',
  });
  const selectedDataFlowId = useWatch({
    control: form.control,
    name: 'pipeline_id',
  });
  const selectedBuiltinId = useWatch({
    control: form.control,
    name: 'parser_id',
  });

  const isPipelineMode = parseType === ParseType.Pipeline;
  const selectedPipelineId = isPipelineMode
    ? selectedDataFlowId
    : selectedBuiltinId;
  // The saved parser_config only belongs to the pipeline (and parse type) it
  // was saved with — expose its id only while that exact pipeline is selected,
  // so that after switching, defaults come purely from the new pipeline's DSL.
  const savedParseType = pipelineId ? ParseType.Pipeline : ParseType.BuiltIn;
  const savedPipelineId =
    parseType === savedParseType
      ? isPipelineMode
        ? pipelineId
        : parserId
      : undefined;

  const { operatorNodes, loading: operatorNodesLoading } =
    usePipelineOperatorNodes(
      selectedPipelineId,
      selectedPipelineId && selectedPipelineId === savedPipelineId
        ? parserConfig
        : undefined,
      !isPipelineMode,
    );

  useResetParserConfigOnPipelineChange(
    form,
    selectedPipelineId,
    savedPipelineId,
    operatorNodes,
  );

  const { activeTab, setActiveTab } = useActiveTab(operatorNodes);

  useEffect(() => {
    if (parseType === ParseType.BuiltIn) {
      form.setValue('pipeline_id', '');
    }
  }, [parseType, form]);

  const handleOperatorValuesChange = useCallback(
    (operatorId: string, values: any) => {
      const currentParserConfig = form.getValues('parser_config') || {};
      // Skip no-op syncs (e.g. a remounted tab pushing back the values it was
      // just initialized with) — otherwise each setValue re-renders the tabs,
      // which re-fires the operator form's change callback in a loop.
      if (isEqual(currentParserConfig[operatorId], values)) {
        return;
      }
      form.setValue('parser_config', {
        ...currentParserConfig,
        [operatorId]: values,
      });
    },
    [form],
  );

  const operatorValues = useWatch({
    control: form.control,
    name: 'parser_config',
  });

  const showOperatorTabs =
    operatorNodes.length > 0 &&
    ((parseType === ParseType.Pipeline && !!selectedDataFlowId) ||
      (parseType === ParseType.BuiltIn && !!selectedBuiltinId));

  const buildSubmitData = useCallback(
    (data: z.infer<typeof FormSchema>): IChangeParserRequestBody => {
      const transformedConfig: Record<string, any> = {};
      for (const [operatorId, config] of Object.entries(
        data.parser_config ?? {},
      )) {
        transformedConfig[operatorId] = transformFormConfigToApi(
          getOperatorType(operatorId),
          config as Record<string, any>,
        );
      }

      const isPipeline = data.parseType === ParseType.Pipeline;
      return {
        parser_id: isPipeline ? '' : data.parser_id,
        pipeline_id: isPipeline ? data.pipeline_id : '',
        parser_config: transformedConfig,
        parseType: data.parseType,
      };
    },
    [],
  );

  return {
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
  };
}
