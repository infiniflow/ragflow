import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useRemoveKnowledgeGraph } from '@/hooks/use-knowledge-request';
import { useCallback } from 'react';
import { useParams } from 'react-router';

export function useDeleteKnowledgeGraph() {
  const { removeKnowledgeGraph, loading } = useRemoveKnowledgeGraph();
  const { navigateToDataset } = useNavigatePage();
  const { id } = useParams();

  const handleDeleteKnowledgeGraph = useCallback(async () => {
    const ret = await removeKnowledgeGraph();
    if (ret === 0 && id) {
      navigateToDataset(id)();
    }
  }, [id, navigateToDataset, removeKnowledgeGraph]);

  return { handleDeleteKnowledgeGraph, loading };
}
