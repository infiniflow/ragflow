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
import { useCallback, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { buildParserSelectValue } from '@/hooks/use-parser-options';
import { useParserTreeData } from '../parser-tree-select/use-parser-tree-data';
import { usePrefillLatestPipelineVersion } from '../parser-tree-select/use-prefill-latest-pipeline-version';
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
  const selectedVersionId = useWatch({
    control: form.control,
    name: 'pipeline_version_id',
  });

  const parserSelectValue = buildParserSelectValue({
    parseType,
    parserId: selectedBuiltinId,
    pipelineId: selectedDataFlowId,
    pipelineVersionId: selectedVersionId,
  });

  // The merged parser tree owns the option data; reusing its existence
  // verdict (same React Query cache, no extra fetches) flags a saved parser
  // or pinned pipeline version that has since been deleted. The verdict
  // drives the dialog display and submit guard directly — it never enters
  // RHF's error lifecycle, so it can never go stale after the user picks a
  // valid option (programmatic setValue does not revalidate).
  const { selectedLeafExists, loading: parserOptionsLoading } =
    useParserTreeData({ value: parserSelectValue });
  const parserOptionMissing =
    !!parserSelectValue && !parserOptionsLoading && !selectedLeafExists;

  const isPipelineMode = parseType === ParseType.Pipeline;
  const selectedPipelineId = isPipelineMode
    ? selectedDataFlowId
    : selectedBuiltinId;
  // The canvas version list drives the latest-version prefill and the D1/D2/D3
  // non-latest indicator. We fetch it inside the hook so the latest id is a
  // *reactive* value: it resolves after mount and the adoption effect below
  // re-fires when it does. (Reading it from a static prop captured before the
  // fetch would freeze it at undefined.) versions is newest-first.
  const { data: versions = [] } = useFetchCanvasVersionList(
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
  const savedVersionId = isPipelineMode ? (pipelineVersionId ?? '') : '';

  // The saved parser_config overrides the DSL defaults only while the
  // selection matches the saved pipeline+version binding; a different version
  // renders that version's own DSL defaults. (The form's parser_config stays
  // the display source of truth via operatorValues either way.)
  const isSavedBindingSelected =
    !!selectedPipelineId &&
    selectedPipelineId === savedPipelineId &&
    (selectedVersionId ?? '') === savedVersionId;

  const { operatorNodes, loading: operatorNodesLoading } =
    usePipelineOperatorNodes({
      pipelineId: selectedPipelineId,
      versionId: isPipelineMode ? selectedVersionId : undefined,
      savedParserConfig: isSavedBindingSelected ? parserConfig : undefined,
      isBuiltin: !isPipelineMode,
    });

  useResetParserConfigOnPipelineChange({
    form,
    pipelineId: selectedPipelineId,
    versionId: isPipelineMode ? selectedVersionId : undefined,
    savedPipelineId,
    savedVersionId: isPipelineMode ? savedVersionId : undefined,
    operatorNodes,
  });

  // Design section 8 (T37): an unpinned document still pre-selects the latest
  // concrete version once the version list resolves. The "switch pipeline
  // clears the pinned version" sync (design 5/B4) needs no effect here: the
  // merged parser tree select writes pipeline_id and pipeline_version_id
  // atomically on every selection, so a stale version from another canvas
  // cannot survive a switch.
  usePrefillLatestPipelineVersion(form, pipelineVersionId, latestVersionId);

  const { activeTab, setActiveTab } = useActiveTab(operatorNodes);

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
  };
}
