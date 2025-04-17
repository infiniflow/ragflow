import { ITestRetrievalRequestBody } from '@/interfaces/request/knowledge';
import kbService from '@/services/knowledge-service';
import { useQuery } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { useParams } from 'umi';
import { useSetPaginationParams } from './route-hook';

export const enum KnowledgeApiAction {
  TestRetrieval = 'testRetrieval',
}

export const useKnowledgeBaseId = () => {
  const { id } = useParams();

  return id;
};

export const useTestRetrieval = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const { page, size: pageSize } = useSetPaginationParams();
  const [values, setValues] = useState<ITestRetrievalRequestBody>();

  const queryParams = useMemo(() => {
    return {
      ...values,
      kb_id: values?.kb_id || knowledgeBaseId,
      page,
      size: pageSize,
    };
  }, [knowledgeBaseId, page, pageSize, values]);

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<any>({
    queryKey: [KnowledgeApiAction.TestRetrieval, queryParams],
    initialData: {},
    // enabled: !!values?.question && !!knowledgeBaseId,
    enabled: false,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await kbService.retrieval_test(queryParams);
      return data?.data ?? {};
    },
  });

  return { data, loading, setValues, refetch };
};
