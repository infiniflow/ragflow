import { ParseType } from '@/constants/knowledge';
import {
  useActiveTab,
  usePipelineOperatorNodes,
  useResetParserConfigOnPipelineChange,
} from '@/hooks/use-pipeline-operator';
import { IChangeParserRequestBody } from '@/interfaces/request/document';
import {
  getOperatorType,
  transformApiConfigToForm,
  transformFormConfigToApi,
} from '@/utils/pipeline-operator';
import { zodResolver } from '@hookform/resolvers/zod';
import { useCallback, useEffect, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

export interface IDocumentPipelineDialogProps {
  parserId: string;
  pipelineId?: string;
  parserConfig?: Record<string, any>;
}

/**
 * Converts the saved parser_config (API format, keyed by operator id) to the
 * form format expected by the operator tabs, mirroring
 * useFetchDatasetSettingOnMount on the dataset setting page.
 */
function transformSavedParserConfigToForm(
  parserConfig?: Record<string, any>,
): Record<string, any> {
  if (
    !parserConfig ||
    typeof parserConfig !== 'object' ||
    Array.isArray(parserConfig)
  ) {
    return {};
  }

  const hasPipelineKeys = Object.keys(parserConfig).some((key) =>
    key.includes(':'),
  );
  if (!hasPipelineKeys) {
    return parserConfig;
  }

  const formParserConfig: Record<string, any> = {};
  for (const [operatorId, config] of Object.entries(parserConfig)) {
    formParserConfig[operatorId] = transformApiConfigToForm(
      getOperatorType(operatorId),
      config as Record<string, any>,
    );
  }
  return formParserConfig;
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
      form.setValue('parser_config', {
        ...currentParserConfig,
        [operatorId]: values,
      });
    },
    [form],
  );

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
    showOperatorTabs,
    buildSubmitData,
  };
}
