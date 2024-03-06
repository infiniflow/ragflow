import showDeleteConfirm from '@/components/deleting-confirm';
import { KnowledgeSearchParams } from '@/constants/knowledge';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { useCallback, useEffect, useMemo } from 'react';
import { useDispatch, useSearchParams, useSelector } from 'umi';

export const useKnowledgeBaseId = (): string => {
  const [searchParams] = useSearchParams();
  const knowledgeBaseId = searchParams.get('id');

  return knowledgeBaseId || '';
};

export const useGetKnowledgeSearchParams = () => {
  const [currentQueryParameters] = useSearchParams();

  return {
    documentId:
      currentQueryParameters.get(KnowledgeSearchParams.DocumentId) || '',
    knowledgeId:
      currentQueryParameters.get(KnowledgeSearchParams.KnowledgeId) || '',
  };
};

export const useDeleteDocumentById = (): {
  removeDocument: (documentId: string) => Promise<number>;
} => {
  const dispatch = useDispatch();
  const knowledgeBaseId = useKnowledgeBaseId();

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
    [removeChunk],
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

export const useFetchKnowledgeList = (
  shouldFilterListWithoutDocument: boolean = false,
): IKnowledge[] => {
  const dispatch = useDispatch();

  const knowledgeModel = useSelector((state: any) => state.knowledgeModel);
  const { data = [] } = knowledgeModel;
  const list = useMemo(() => {
    return shouldFilterListWithoutDocument
      ? data.filter((x: IKnowledge) => x.doc_num > 0)
      : data;
  }, [data, shouldFilterListWithoutDocument]);

  const fetchList = useCallback(() => {
    dispatch({
      type: 'knowledgeModel/getList',
    });
  }, [dispatch]);

  useEffect(() => {
    fetchList();
  }, [fetchList]);

  return list;
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
