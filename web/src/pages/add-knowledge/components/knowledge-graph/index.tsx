import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { useFetchKnowledgeGraph } from '@/hooks/knowledge-hooks';
import { Trash2 } from 'lucide-react';
import React from 'react';
import { useTranslation } from 'react-i18next';
import ForceGraph from './force-graph';
import { useDeleteKnowledgeGraph } from './use-delete-graph';

const KnowledgeGraph: React.FC = () => {
  const { data } = useFetchKnowledgeGraph();
  const { t } = useTranslation();
  const { handleDeleteKnowledgeGraph } = useDeleteKnowledgeGraph();

  const totalNodes = data?.graph?.total_nodes || 0;
  const totalEdges = data?.graph?.total_edges || 0;
  const displayedNodes = data?.graph?.nodes?.length || 0;
  const displayedEdges = data?.graph?.edges?.length || 0;

  return (
    <section className={'w-full h-full relative'}>
      <ConfirmDeleteDialog onOk={handleDeleteKnowledgeGraph}>
        <Button
          variant="outline"
          size={'sm'}
          className="absolute right-0 top-0 z-50"
        >
          <Trash2 /> {t('common.delete')}
        </Button>
      </ConfirmDeleteDialog>
      
      {/* Graph Statistics */}
      <div className="absolute left-4 top-4 z-50 bg-white/90 backdrop-blur-sm rounded-lg p-3 shadow-md border">
        <div className="text-sm font-medium text-gray-700 mb-2">
          {t('knowledgeGraph.statistics', 'Graph Statistics')}
        </div>
        <div className="space-y-1 text-xs text-gray-600">
          <div className="flex justify-between gap-4">
            <span>{t('knowledgeGraph.nodes', 'Nodes')}:</span>
            <span className="font-mono">
              {displayedNodes.toLocaleString()} / {totalNodes.toLocaleString()}
            </span>
          </div>
          <div className="flex justify-between gap-4">
            <span>{t('knowledgeGraph.edges', 'Edges')}:</span>
            <span className="font-mono">
              {displayedEdges.toLocaleString()} / {totalEdges.toLocaleString()}
            </span>
          </div>
        </div>
      </div>
      
      <ForceGraph data={data?.graph} show></ForceGraph>
    </section>
  );
};

export default KnowledgeGraph;
