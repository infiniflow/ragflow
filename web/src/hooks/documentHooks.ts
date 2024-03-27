import { IChunk, IKnowledgeFile } from '@/interfaces/database/knowledge';
import { IChangeParserConfigRequestBody } from '@/interfaces/request/document';
import { api_host } from '@/utils/api';
import { buildChunkHighlights } from '@/utils/documentUtils';
import { UploadFile } from 'antd';
import { useCallback, useMemo, useState } from 'react';
import { IHighlight } from 'react-pdf-highlighter';
import { useDispatch, useSelector } from 'umi';
import { useGetKnowledgeSearchParams } from './routeHook';

export const useGetDocumentUrl = (documentId: string) => {
  const url = useMemo(() => {
    return `${api_host}/document/get/${documentId}`;
  }, [documentId]);

  return url;
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

export const useRemoveDocument = (documentId: string) => {
  const dispatch = useDispatch();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const removeDocument = useCallback(() => {
    try {
      return dispatch<any>({
        type: 'kFModel/document_rm',
        payload: {
          doc_id: documentId,
          kb_id: knowledgeId,
        },
      });
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  }, [dispatch, knowledgeId, documentId]);

  return removeDocument;
};

export const useUploadDocument = () => {
  const dispatch = useDispatch();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const uploadDocument = useCallback(
    (file: UploadFile) => {
      try {
        return dispatch<any>({
          type: 'kFModel/upload_document',
          payload: {
            file,
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

export const useRunDocument = () => {
  const dispatch = useDispatch();

  const runDocumentByIds = useCallback(
    (ids: string[]) => {
      try {
        return dispatch<any>({
          type: 'kFModel/document_run',
          payload: { doc_ids: ids, run: 1 },
        });
      } catch (errorInfo) {
        console.log('Failed:', errorInfo);
      }
    },
    [dispatch],
  );

  return runDocumentByIds;
};
