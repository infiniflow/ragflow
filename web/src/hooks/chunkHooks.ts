import { useCallback } from 'react';
import { useDispatch } from 'umi';
import { useGetKnowledgeSearchParams } from './routeHook';

interface PayloadType {
  doc_id: string;
  keywords?: string;
}

export const useFetchChunkList = () => {
  const dispatch = useDispatch();
  const { documentId } = useGetKnowledgeSearchParams();

  const fetchChunkList = useCallback(() => {
    dispatch({
      type: 'chunkModel/chunk_list',
      payload: {
        doc_id: documentId,
      },
    });
  }, [dispatch, documentId]);

  return fetchChunkList;
};
