import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { IConnector } from '@/interfaces/database/dataset';
import { useDataSourceInfo } from '@/pages/user-setting/data-source/constant';
import { checkEmbedding } from '@/services/knowledge-service';
import { pick } from 'lodash';
import { useCallback, useEffect, useMemo } from 'react';
import { UseFormReturn } from 'react-hook-form';
import { useParams, useSearchParams } from 'react-router';
import { z } from 'zod';
import { formSchema } from './form-schema';

export function useHasParsedDocument(isEdit?: boolean) {
  const { data: knowledgeDetails } = useFetchKnowledgeBaseConfiguration({
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
    useFetchKnowledgeBaseConfiguration();
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
    const formValues = {
      ...pick(knowledgeDetails, [
        'description',
        'name',
        'permission',
        'connectors',
        'pagerank',
        'avatar',
      ]),
      embedding_model: knowledgeDetails.embedding_model,
      connectors: sourceData,
    } as z.infer<typeof formSchema>;
    form.reset(formValues);
  }, [form, knowledgeDetails, sourceData]);

  return { knowledgeDetails, loading };
};
