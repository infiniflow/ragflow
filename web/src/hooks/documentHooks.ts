import { IChunk, IKnowledgeFile } from '@/interfaces/database/knowledge';
import { api_host } from '@/utils/api';
import { buildChunkHighlights } from '@/utils/documentUtils';
import { useCallback, useMemo } from 'react';
import { IHighlight } from 'react-pdf-highlighter';
import { useDispatch, useSelector } from 'umi';
import { useGetKnowledgeSearchParams } from './routeHook';

export const useGetDocumentUrl = (documentId: string) => {
  const url = useMemo(() => {
    return `${api_host}/document/get/${documentId}`;
  }, [documentId]);

  return url;
};

export const useGetChunkHighlights = (selectedChunk: IChunk): IHighlight[] => {
  const highlights: IHighlight[] = useMemo(() => {
    return buildChunkHighlights(selectedChunk);
  }, [selectedChunk]);

  return highlights;
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
