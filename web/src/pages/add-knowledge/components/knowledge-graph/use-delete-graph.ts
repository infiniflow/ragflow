import {
  useKnowledgeBaseId,
  useRemoveKnowledgeGraph,
} from '@/hooks/knowledge-hooks';
import { useCallback } from 'react';
import { useNavigate } from 'umi';

export function useDeleteKnowledgeGraph() {
  const { removeKnowledgeGraph, loading } = useRemoveKnowledgeGraph();
  const navigate = useNavigate();
  const knowledgeBaseId = useKnowledgeBaseId();

  const handleDeleteKnowledgeGraph = useCallback(async () => {
    const ret = await removeKnowledgeGraph();
    if (ret === 0) {
      navigate(`/knowledge/dataset?id=${knowledgeBaseId}`);
    }
  }, [knowledgeBaseId, navigate, removeKnowledgeGraph]);

  return { handleDeleteKnowledgeGraph, loading };
}
