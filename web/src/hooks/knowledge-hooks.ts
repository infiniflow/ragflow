import { useShowDeleteConfirm } from '@/hooks/common-hooks';
import { IKnowledge } from '@/interfaces/database/knowledge';
import i18n from '@/locales/config';
import kbService from '@/services/knowledge-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { message } from 'antd';
import { useCallback, useEffect } from 'react';
import { useDispatch, useSearchParams, useSelector } from 'umi';
import { useGetKnowledgeSearchParams } from './route-hook';

export const useKnowledgeBaseId = (): string => {
  const [searchParams] = useSearchParams();
  const knowledgeBaseId = searchParams.get('id');

  return knowledgeBaseId || '';
};

export const useDeleteDocumentById = (): {
  removeDocument: (documentId: string) => Promise<number>;
} => {
  const dispatch = useDispatch();
  const knowledgeBaseId = useKnowledgeBaseId();
  const showDeleteConfirm = useShowDeleteConfirm();

  const removeDocument = (documentId: string) => () => {
    return dispatch({
      type: 'kFModel/document_rm',
      payload: {
        doc_id: documentId,
        kb_id: knowledgeBaseId,
      },
    });
  };

  const onRmDocument = (documentId: string): Promise<number> => {
    return showDeleteConfirm({ onOk: removeDocument(documentId) });
  };

  return {
    removeDocument: onRmDocument,
  };
};

export const useFetchKnowledgeDetail = () => {
  const dispatch = useDispatch();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const fetchKnowledgeDetail = useCallback(
    (knowledgeId: string) => {
      dispatch({
        type: 'knowledgeModel/getKnowledgeDetail',
        payload: { kb_id: knowledgeId },
      });
    },
    [dispatch],
  );

  useEffect(() => {
    fetchKnowledgeDetail(knowledgeId);
  }, [fetchKnowledgeDetail, knowledgeId]);

  return fetchKnowledgeDetail;
};

export const useSelectKnowledgeDetail = () => {
  const knowledge: IKnowledge = useSelector(
    (state: any) => state.knowledgeModel.knowledge,
  );

  return knowledge;
};

export const useGetDocumentDefaultParser = () => {
  const item = useSelectKnowledgeDetail();

  return {
    defaultParserId: item?.parser_id ?? '',
    parserConfig: item?.parser_config ?? '',
  };
};

export const useDeleteChunkByIds = (): {
  removeChunk: (chunkIds: string[], documentId: string) => Promise<number>;
} => {
  const dispatch = useDispatch();
  const showDeleteConfirm = useShowDeleteConfirm();

  const removeChunk = useCallback(
    (chunkIds: string[], documentId: string) => () => {
      return dispatch({
        type: 'chunkModel/rm_chunk',
        payload: {
          chunk_ids: chunkIds,
          doc_id: documentId,
        },
      });
    },
    [dispatch],
  );

  const onRemoveChunk = useCallback(
    (chunkIds: string[], documentId: string): Promise<number> => {
      return showDeleteConfirm({ onOk: removeChunk(chunkIds, documentId) });
    },
    [removeChunk, showDeleteConfirm],
  );

  return {
    removeChunk: onRemoveChunk,
  };
};

export const useFetchKnowledgeBaseConfiguration = () => {
  const dispatch = useDispatch();
  const knowledgeBaseId = useKnowledgeBaseId();

  const fetchKnowledgeBaseConfiguration = useCallback(() => {
    dispatch({
      type: 'kSModel/getKbDetail',
      payload: {
        kb_id: knowledgeBaseId,
      },
    });
  }, [dispatch, knowledgeBaseId]);

  useEffect(() => {
    fetchKnowledgeBaseConfiguration();
  }, [fetchKnowledgeBaseConfiguration]);
};

export const useNextFetchKnowledgeList = (
  shouldFilterListWithoutDocument: boolean = false,
): {
  list: any[];
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

export const useSelectFileThumbnails = () => {
  const fileThumbnails: Record<string, string> = useSelector(
    (state: any) => state.kFModel.fileThumbnails,
  );

  return fileThumbnails;
};

export const useFetchFileThumbnails = (docIds?: Array<string>) => {
  const dispatch = useDispatch();
  const fileThumbnails = useSelectFileThumbnails();

  const fetchFileThumbnails = useCallback(
    (docIds: Array<string>) => {
      dispatch({
        type: 'kFModel/fetch_document_thumbnails',
        payload: { doc_ids: docIds.join(',') },
      });
    },
    [dispatch],
  );

  useEffect(() => {
    if (docIds) {
      fetchFileThumbnails(docIds);
    }
  }, [docIds, fetchFileThumbnails]);

  return { fileThumbnails, fetchFileThumbnails };
};

//#region knowledge configuration

export const useUpdateKnowledge = () => {
  const dispatch = useDispatch();

  const saveKnowledgeConfiguration = useCallback(
    (payload: any) => {
      dispatch({
        type: 'kSModel/updateKb',
        payload,
      });
    },
    [dispatch],
  );

  return saveKnowledgeConfiguration;
};

export const useSelectKnowledgeDetails = () => {
  const knowledgeDetails: IKnowledge = useSelector(
    (state: any) => state.kSModel.knowledgeDetails,
  );
  return knowledgeDetails;
};
//#endregion

//#region Retrieval testing

export const useTestChunkRetrieval = () => {
  const dispatch = useDispatch();
  const knowledgeBaseId = useKnowledgeBaseId();

  const testChunk = useCallback(
    (values: any) => {
      dispatch({
        type: 'testingModel/testDocumentChunk',
        payload: {
          ...values,
          kb_id: knowledgeBaseId,
        },
      });
    },
    [dispatch, knowledgeBaseId],
  );

  return testChunk;
};
//#endregion
