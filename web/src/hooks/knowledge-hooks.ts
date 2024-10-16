import { ResponsePostType } from '@/interfaces/database/base';
import { IKnowledge, ITestingResult } from '@/interfaces/database/knowledge';
import i18n from '@/locales/config';
import kbService from '@/services/knowledge-service';
import {
  useIsMutating,
  useMutation,
  useMutationState,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { message } from 'antd';
import { useSearchParams } from 'umi';
import { useSetPaginationParams } from './route-hook';

export const useKnowledgeBaseId = (): string => {
  const [searchParams] = useSearchParams();
  const knowledgeBaseId = searchParams.get('id');

  return knowledgeBaseId || '';
};

export const useFetchKnowledgeBaseConfiguration = () => {
  const knowledgeBaseId = useKnowledgeBaseId();

  const { data, isFetching: loading } = useQuery({
    queryKey: ['fetchKnowledgeDetail'],
    initialData: {},
    gcTime: 0,
    queryFn: async () => {
      const { data } = await kbService.get_kb_detail({
        kb_id: knowledgeBaseId,
      });
      return data?.data ?? {};
    },
  });

  return { data, loading };
};

export const useNextFetchKnowledgeList = (
  shouldFilterListWithoutDocument: boolean = false,
): {
  list: IKnowledge[];
  loading: boolean;
} => {
  const { data, isFetching: loading } = useQuery({
    queryKey: ['fetchKnowledgeList'],
    initialData: [],
    gcTime: 0, // https://tanstack.com/query/latest/docs/framework/react/guides/caching?from=reactQueryV3
    queryFn: async () => {
      const { data } = await kbService.getList();
      const list = data?.data ?? [];
      return shouldFilterListWithoutDocument
        ? list.filter((x: IKnowledge) => x.chunk_num > 0)
        : list;
    },
  });

  return { list: data, loading };
};

export const useCreateKnowledge = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['createKnowledge'],
    mutationFn: async (params: { id?: string; name: string }) => {
      const { data = {} } = await kbService.createKb(params);
      if (data.retcode === 0) {
        message.success(
          i18n.t(`message.${params?.id ? 'modified' : 'created'}`),
        );
        queryClient.invalidateQueries({ queryKey: ['fetchKnowledgeList'] });
      }
      return data;
    },
  });

  return { data, loading, createKnowledge: mutateAsync };
};

export const useDeleteKnowledge = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteKnowledge'],
    mutationFn: async (id: string) => {
      const { data } = await kbService.rmKb({ kb_id: id });
      if (data.retcode === 0) {
        message.success(i18n.t(`message.deleted`));
        queryClient.invalidateQueries({ queryKey: ['fetchKnowledgeList'] });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, deleteKnowledge: mutateAsync };
};

//#region knowledge configuration

export const useUpdateKnowledge = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['saveKnowledge'],
    mutationFn: async (params: Record<string, any>) => {
      const { data = {} } = await kbService.updateKb({
        kb_id: knowledgeBaseId,
        ...params,
      });
      if (data.retcode === 0) {
        message.success(i18n.t(`message.updated`));
        queryClient.invalidateQueries({ queryKey: ['fetchKnowledgeDetail'] });
      }
      return data;
    },
  });

  return { data, loading, saveKnowledgeConfiguration: mutateAsync };
};

//#endregion

//#region Retrieval testing

export const useTestChunkRetrieval = (): ResponsePostType<ITestingResult> & {
  testChunk: (...params: any[]) => void;
} => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const { page, size: pageSize } = useSetPaginationParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['testChunk'], // This method is invalid
    gcTime: 0,
    mutationFn: async (values: any) => {
      const { data } = await kbService.retrieval_test({
        ...values,
        kb_id: values.kb_id ?? knowledgeBaseId,
        page,
        size: pageSize,
      });
      if (data.retcode === 0) {
        const res = data.data;
        return {
          chunks: res.chunks,
          documents: res.doc_aggs,
          total: res.total,
        };
      }
      return (
        data?.data ?? {
          chunks: [],
          documents: [],
          total: 0,
        }
      );
    },
  });

  return {
    data: data ?? { chunks: [], documents: [], total: 0 },
    loading,
    testChunk: mutateAsync,
  };
};

export const useChunkIsTesting = () => {
  return useIsMutating({ mutationKey: ['testChunk'] }) > 0;
};

export const useSelectTestingResult = (): ITestingResult => {
  const data = useMutationState({
    filters: { mutationKey: ['testChunk'] },
    select: (mutation) => {
      return mutation.state.data;
    },
  });
  return (data.at(-1) ?? {
    chunks: [],
    documents: [],
    total: 0,
  }) as ITestingResult;
};
//#endregion
