import { IChunk, IKnowledgeFile } from '@/interfaces/database/knowledge';
import { IChangeParserConfigRequestBody } from '@/interfaces/request/document';
import { api_host } from '@/utils/api';
import { buildChunkHighlights } from '@/utils/document-util';
import { UploadFile } from 'antd';
import { useCallback, useMemo, useState } from 'react';
import { IHighlight } from 'react-pdf-highlighter';
import { useDispatch, useSelector } from 'umi';
import { useGetKnowledgeSearchParams } from './route-hook';
import { useOneNamespaceEffectsLoading } from './store-hooks';

export const useGetDocumentUrl = (documentId?: string) => {
  const getDocumentUrl = useCallback(
    (id?: string) => {
      return `${api_host}/document/get/${documentId || id}`;
    },
    [documentId],
  );

  return getDocumentUrl;
};

export const useGetChunkHighlights = (selectedChunk: IChunk) => {
  const [size, setSize] = useState({ width: 849, height: 1200 });

  const highlights: IHighlight[] = useMemo(() => {
    return buildChunkHighlights(selectedChunk, size);
  }, [selectedChunk, size]);

  const setWidthAndHeight = (width: number, height: number) => {
    setSize((pre) => {
      if (pre.height !== height || pre.width !== width) {
        return { height, width };
      }
      return pre;
    });
  };

  return { highlights, setWidthAndHeight };
};

export const useFetchDocumentList = () => {
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const dispatch = useDispatch();

  const fetchKfList = useCallback(() => {
    return dispatch<any>({
      type: 'kFModel/getKfList',
      payload: {
        kb_id: knowledgeId,
      },
    });
  }, [dispatch, knowledgeId]);

  return fetchKfList;
};

export const useSetDocumentStatus = () => {
  const dispatch = useDispatch();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const setDocumentStatus = useCallback(
    (status: boolean, documentId: string) => {
      dispatch({
        type: 'kFModel/updateDocumentStatus',
        payload: {
          doc_id: documentId,
          status: Number(status),
          kb_id: knowledgeId,
        },
      });
    },
    [dispatch, knowledgeId],
  );

  return setDocumentStatus;
};

export const useSelectDocumentList = () => {
  const list: IKnowledgeFile[] = useSelector(
    (state: any) => state.kFModel.data,
  );
  return list;
};

export const useSaveDocumentName = () => {
  const dispatch = useDispatch();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const saveName = useCallback(
    (documentId: string, name: string) => {
      return dispatch<any>({
        type: 'kFModel/document_rename',
        payload: {
          doc_id: documentId,
          name: name,
          kb_id: knowledgeId,
        },
      });
    },
    [dispatch, knowledgeId],
  );

  return saveName;
};

export const useCreateDocument = () => {
  const dispatch = useDispatch();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const createDocument = useCallback(
    (name: string) => {
      try {
        return dispatch<any>({
          type: 'kFModel/document_create',
          payload: {
            name,
            kb_id: knowledgeId,
          },
        });
      } catch (errorInfo) {
        console.log('Failed:', errorInfo);
      }
    },
    [dispatch, knowledgeId],
  );

  return createDocument;
};

export const useSetDocumentParser = () => {
  const dispatch = useDispatch();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const setDocumentParser = useCallback(
    (
      parserId: string,
      documentId: string,
      parserConfig: IChangeParserConfigRequestBody,
    ) => {
      try {
        return dispatch<any>({
          type: 'kFModel/document_change_parser',
          payload: {
            parser_id: parserId,
            doc_id: documentId,
            kb_id: knowledgeId,
            parser_config: parserConfig,
          },
        });
      } catch (errorInfo) {
        console.log('Failed:', errorInfo);
      }
    },
    [dispatch, knowledgeId],
  );

  return setDocumentParser;
};

export const useRemoveDocument = () => {
  const dispatch = useDispatch();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const removeDocument = useCallback(
    (documentIds: string[]) => {
      try {
        return dispatch<any>({
          type: 'kFModel/document_rm',
          payload: {
            doc_id: documentIds,
            kb_id: knowledgeId,
          },
        });
      } catch (errorInfo) {
        console.log('Failed:', errorInfo);
      }
    },
    [dispatch, knowledgeId],
  );

  return removeDocument;
};

export const useUploadDocument = () => {
  const dispatch = useDispatch();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const uploadDocument = useCallback(
    (fileList: UploadFile[]) => {
      try {
        return dispatch<any>({
          type: 'kFModel/upload_document',
          payload: {
            fileList,
            kb_id: knowledgeId,
          },
        });
      } catch (errorInfo) {
        console.log('Failed:', errorInfo);
      }
    },
    [dispatch, knowledgeId],
  );

  return uploadDocument;
};

export const useWebCrawl = () => {
  const dispatch = useDispatch();
  const { knowledgeId } = useGetKnowledgeSearchParams();
  return useCallback(
    (name: string, url: string) => {
      try {
        return dispatch<any>({
          type: 'kFModel/web_crawl',
          payload: {
            name,
            url,
            kb_id: knowledgeId,
          },
        });
      } catch (errorInfo) {
        console.log('Failed:', errorInfo);
      }
    },
    [dispatch],
  );
};

export const useRunDocument = () => {
  const dispatch = useDispatch();

  const runDocumentByIds = useCallback(
    (payload: any) => {
      try {
        return dispatch<any>({
          type: 'kFModel/document_run',
          payload,
        });
      } catch (errorInfo) {
        console.log('Failed:', errorInfo);
      }
    },
    [dispatch],
  );

  return runDocumentByIds;
};

export const useSelectRunDocumentLoading = () => {
  const loading = useOneNamespaceEffectsLoading('kFModel', ['document_run']);
  return loading;
};
