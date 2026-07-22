import { ParseType } from '@/constants/knowledge';
import {
  useFetchDatasetPipelineConfiguration,
  useUpdateKnowledge,
} from '@/hooks/use-knowledge-request';
import { IConnector } from '@/interfaces/database/dataset';
import { useDataSourceInfo } from '@/pages/user-setting/data-source/constant';
import { checkEmbedding } from '@/services/knowledge-service';
import {
  getOperatorType,
  transformApiConfigToForm,
  transformFormConfigToApi,
} from '@/utils/pipeline-operator';
import { pick } from 'lodash';
import {
  Dispatch,
  SetStateAction,
  useCallback,
  useEffect,
  useMemo,
} from 'react';
import { UseFormReturn } from 'react-hook-form';
import { useParams, useSearchParams } from 'react-router';
import { z } from 'zod';
import { formSchema } from './form-schema';

export function useHasParsedDocument(isEdit?: boolean) {
  const { data: knowledgeDetails } = useFetchDatasetPipelineConfiguration({
    isEdit,
  });
  return knowledgeDetails.chunk_count > 0;
}

export const useHandleKbEmbedding = () => {
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const knowledgeBaseId = searchParams.get('id') || id;
  const handleChange = useCallback(
    async ({ embed_id }: { embed_id: string }) => {
      const res = await checkEmbedding(knowledgeBaseId || '', {
        embd_id: embed_id,
      });
      return res.data;
    },
    [knowledgeBaseId],
  );
  return {
    handleChange,
  };
};

export const useFetchDatasetSettingOnMount = (
  form: UseFormReturn<z.infer<typeof formSchema>>,
) => {
  const { data: knowledgeDetails, loading } =
    useFetchDatasetPipelineConfiguration();
  const { dataSourceInfo } = useDataSourceInfo();

  const sourceData = useMemo(() => {
    return (knowledgeDetails?.connectors ?? []).map(
      (connector: IConnector) => ({
        ...connector,
        icon:
          dataSourceInfo[connector.source as keyof typeof dataSourceInfo]
            ?.icon || '',
      }),
    );
  }, [knowledgeDetails?.connectors, dataSourceInfo]);

  useEffect(() => {
    const parserConfig = knowledgeDetails.parser_config as
      | Record<string, any>
      | undefined;
    let formParserConfig: Record<string, any> | undefined = parserConfig;

    // Convert parser_config to form format if in pipeline mode
    if (
      parserConfig &&
      typeof parserConfig === 'object' &&
      !Array.isArray(parserConfig)
    ) {
      const keys = Object.keys(parserConfig);
      const hasPipelineKeys = keys.some((key) => key.includes(':'));
      if (hasPipelineKeys) {
        formParserConfig = {};
        for (const [operatorId, config] of Object.entries(parserConfig)) {
          const operatorType = getOperatorType(operatorId);
          formParserConfig[operatorId] = transformApiConfigToForm(
            operatorType,
            config as Record<string, any>,
          );
        }
      }
    }

    const formValues = {
      ...pick(knowledgeDetails, [
        'description',
        'name',
        'permission',
        'connectors',
        'pagerank',
        'avatar',
        'pipeline_id',
        'pipeline_name',
        'pipeline_avatar',
        'parser_id',
      ]),
      embedding_model: knowledgeDetails.embedding_model,
      connectors: sourceData,
      parse_type: knowledgeDetails.pipeline_id
        ? ParseType.Pipeline
        : ParseType.BuiltIn,
      parser_config: formParserConfig,
    } as z.infer<typeof formSchema>;
    form.reset(formValues);
  }, [form, knowledgeDetails, sourceData]);

  return { knowledgeDetails, loading, sourceData };
};

export const useSaveDatasetSetting = () => {
  const { saveKnowledgeConfiguration, loading } = useUpdateKnowledge();

  const handleSave = useCallback(
    async (values: z.infer<typeof formSchema>) => {
      const payload = { ...values };

      // Apply forward transforms to parser_config if in pipeline mode
      if (payload.parser_config) {
        const transformedConfig: Record<string, any> = {};
        for (const [operatorId, config] of Object.entries(
          payload.parser_config,
        )) {
          const operatorType = getOperatorType(operatorId);
          transformedConfig[operatorId] = transformFormConfigToApi(
            operatorType,
            config as Record<string, any>,
          );
        }
        payload.parser_config = transformedConfig;
      }

      if (payload.parse_type === ParseType.BuiltIn) {
        payload.pipeline_id = undefined;
      } else {
        payload.parser_id = undefined;
      }
      return saveKnowledgeConfiguration(payload);
    },
    [saveKnowledgeConfiguration],
  );

  return { handleSave, loading };
};

export const usePipelineDataList = (sourceData: any[] | undefined) => {
  const { dataSourceInfo } = useDataSourceInfo();

  return useMemo(
    () =>
      sourceData?.map((item) => ({
        ...item,
        icon:
          dataSourceInfo[item.source as keyof typeof dataSourceInfo]?.icon ||
          '',
      })),
    [sourceData, dataSourceInfo],
  );
};

export const useConnectorHandlers = (
  form: UseFormReturn<any>,
  sourceData?: any[],
  setSourceData?: Dispatch<SetStateAction<any[] | undefined>>,
) => {
  const { dataSourceInfo } = useDataSourceInfo();

  const handleLinkOrEditSubmit = useCallback(
    (data: any[] | undefined) => {
      if (data) {
        const connectors = data.map((connector) => ({
          ...connector,
          auto_parse: (connector as IConnector).auto_parse === '0' ? '0' : '1',
          icon:
            dataSourceInfo[connector.source as keyof typeof dataSourceInfo]
              ?.icon || '',
        }));
        setSourceData?.(connectors);
        form.setValue('connectors', connectors || []);
      }
    },
    [dataSourceInfo, form, setSourceData],
  );

  const unbindFunc = useCallback(
    (data: any) => {
      if (data) {
        const connectors = sourceData?.filter(
          (connector) => connector.id !== data.id,
        );
        setSourceData?.(connectors);
        form.setValue('connectors', connectors || []);
      }
    },
    [sourceData, form, setSourceData],
  );

  const handleAutoParse = useCallback(
    ({
      source_id,
      isAutoParse,
    }: {
      source_id: string;
      isAutoParse: boolean;
    }) => {
      if (source_id) {
        const connectors = sourceData?.map((connector) => {
          if (connector.id === source_id) {
            return {
              ...connector,
              auto_parse: isAutoParse ? '1' : '0',
            };
          }
          return connector;
        });
        setSourceData?.(connectors);
        form.setValue('connectors', connectors || []);
      }
    },
    [sourceData, form, setSourceData],
  );

  return { handleLinkOrEditSubmit, unbindFunc, handleAutoParse };
};
