import { useRemoveKnowledgeGraph } from '@/hooks/knowledge-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useCallback } from 'react';
import { useParams } from 'umi';

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
