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
import { useFetchCanvasVersionList } from '@/hooks/use-agent-request';
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
  // Currently pinned canvas version id (design section 5). Empty when the
  // dataset/document binds the latest version.
  pipelineVersionId?: string;
  parserConfig?: Record<string, any>;
}

export function useDocumentPipelineForm({
  parserId,
  pipelineId,
  pipelineVersionId,
  parserConfig,
}: IDocumentPipelineDialogProps) {
  const { t } = useTranslation();

  const FormSchema = useMemo(
    () =>
      z
        .object({
          parse_type: z.nativeEnum(ParseType),
          // Optional because it is only populated in BuiltIn mode; in Pipeline
          // mode the field's input unmounts and RHF reports it as undefined.
          // The superRefine below enforces it when BuiltIn is selected.
          parser_id: z.string().optional(),
          pipeline_id: z.string().optional(),
          pipeline_version_id: z.string().optional(),
          parser_config: z.record(z.string(), z.any()).optional(),
        })
        .superRefine((data, ctx) => {
          if (
            data.parse_type === ParseType.BuiltIn &&
            !(data.parser_id ?? '').trim()
          ) {
            ctx.addIssue({
              path: ['parser_id'],
              message: t('common.pleaseSelect'),
              code: 'custom',
            });
          }
          if (data.parse_type === ParseType.Pipeline && !data.pipeline_id) {
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
      parse_type: pipelineId ? ParseType.Pipeline : ParseType.BuiltIn,
      parser_id: parserId || '',
      pipeline_id: pipelineId || '',
      // Design section 8: an unpinned document still pre-selects the latest
      // concrete version so the UI shows a specific version, never the
      // "Latest version" placeholder. A pinned value wins when present.
      pipeline_version_id: pipelineVersionId || '',
      parser_config: transformSavedParserConfigToForm(parserConfig),
    },
  });

  const parseType = useWatch({
    control: form.control,
    name: 'parse_type',
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
  // The canvas version list drives the selector, the D1/D2/D3 non-latest `!`
  // indicator and the T29 Missing (unparseable) detection. We fetch it inside
  // the hook so the latest id is a *reactive* value: it resolves after mount
  // and the adoption effect below re-fires when it does. (Reading it from a
  // static prop captured before the fetch would freeze it at undefined.)
  const {
    data: versions = [],
    loading: versionListLoading,
    isError: versionListError,
  } = useFetchCanvasVersionList(
    isPipelineMode ? selectedDataFlowId : undefined,
  );
  const latestVersionId = versions[0]?.id;
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

  // A pinned version belongs to the canvas it was pinned on (design section 5/B4
  // sticky snapshot). Switching the dataflow canvas invalidates any previously
  // pinned version, so clear it; otherwise a version id from the old canvas
  // would be submitted against the new pipeline. The initial load keeps the
  // saved version (selected equals saved).
  useEffect(() => {
    if (selectedPipelineId !== savedPipelineId) {
      form.setValue('pipeline_version_id', '');
    }
  }, [selectedPipelineId, savedPipelineId, form]);

  // Design section 8 (T37): an unpinned document must never show the "Latest
  // version" pseudo-value, so adopt the latest concrete version id once the
  // version list resolves (it loads asynchronously, hence the effect rather
  // than the form default). `latestVersionId` is reactive (derived from the
  // fetch above), so this re-fires when the list arrives. We only adopt while
  // the field is still empty, so a user's manual selection of an older version
  // is preserved and a stable latest id will not re-fire this effect.
  useEffect(() => {
    if (
      !pipelineVersionId &&
      latestVersionId &&
      !form.getValues('pipeline_version_id')
    ) {
      form.setValue('pipeline_version_id', latestVersionId);
    }
  }, [pipelineVersionId, latestVersionId, form]);

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

      const isPipeline = data.parse_type === ParseType.Pipeline;
      return {
        parser_id: isPipeline ? '' : (data.parser_id ?? ''),
        pipeline_id: isPipeline ? data.pipeline_id : '',
        pipeline_version_id: isPipeline ? (data.pipeline_version_id ?? '') : '',
        parser_config: transformedConfig,
        parse_type: data.parse_type,
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
    versions,
    latestVersionId,
    versionListLoading,
    versionListError,
  };
}
