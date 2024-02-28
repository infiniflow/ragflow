import showDeleteConfirm from '@/components/deleting-confirm';
import { IKnowledge, ITenantInfo } from '@/interfaces/database/knowledge';
import { useCallback, useEffect, useMemo } from 'react';
import { useDispatch, useSearchParams, useSelector } from 'umi';

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

export const useGetDocumentDefaultParser = (knowledgeBaseId: string) => {
  const data: IKnowledge[] = useSelector(
    (state: any) => state.knowledgeModel.data,
  );

  const item = data.find((x) => x.id === knowledgeBaseId);

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

export const useSelectParserList = (): Array<{
  value: string;
  label: string;
}> => {
  const tenantIfo: Nullable<ITenantInfo> = useSelector(
    (state: any) => state.settingModel.tenantIfo,
  );

  const parserList = useMemo(() => {
    const parserArray: Array<string> = tenantIfo?.parser_ids.split(',') ?? [];
    return parserArray.map((x) => {
      const arr = x.split(':');
      return { value: arr[0], label: arr[1] };
    });
  }, [tenantIfo]);

  return parserList;
};

export const useFetchParserList = () => {
  const dispatch = useDispatch();

  useEffect(() => {
    dispatch({
      type: 'settingModel/getTenantInfo',
    });
  }, [dispatch]);
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
