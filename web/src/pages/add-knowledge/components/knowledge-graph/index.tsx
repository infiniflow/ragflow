import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { useFetchKnowledgeGraph, useResolveEntities, useDetectCommunities } from '@/hooks/knowledge-hooks';
import { Trash2, Network, Users } from 'lucide-react';
import React from 'react';
import { useTranslation } from 'react-i18next';
import ForceGraph from './force-graph';
import { useDeleteKnowledgeGraph } from './use-delete-graph';

const KnowledgeGraph: React.FC = () => {
  const { data } = useFetchKnowledgeGraph();
  const { t } = useTranslation();
  const { handleDeleteKnowledgeGraph } = useDeleteKnowledgeGraph();
  const { resolveEntities, loading: resolvingEntities } = useResolveEntities();
  const { detectCommunities, loading: detectingCommunities } = useDetectCommunities();

  const totalNodes = data?.graph?.total_nodes || 0;
  const totalEdges = data?.graph?.total_edges || 0;
  const displayedNodes = data?.graph?.nodes?.length || 0;
  const displayedEdges = data?.graph?.edges?.length || 0;

  const handleResolveEntities = async () => {
    try {
      await resolveEntities();
    } catch (error) {
      console.error('Entity resolution failed:', error);
    }
  };

  const handleDetectCommunities = async () => {
    try {
      await detectCommunities();
    } catch (error) {
      console.error('Community detection failed:', error);
    }
  };

  return (
    <section className={'w-full h-full relative'}>
      {/* Action buttons */}
      <div className="absolute right-0 top-0 z-50 flex gap-2">
        <Button
          variant="outline"
          size={'sm'}
          onClick={handleResolveEntities}
          disabled={resolvingEntities || totalNodes === 0}
          className="flex items-center gap-2"
        >
          <Network className="w-4 h-4" />
          {resolvingEntities ? t('knowledgeGraph.resolving', 'Resolving...') : t('knowledgeGraph.resolveEntities', 'Resolve Entities')}
        </Button>
        
        <Button
          variant="outline"
          size={'sm'}
          onClick={handleDetectCommunities}
          disabled={detectingCommunities || totalNodes === 0}
          className="flex items-center gap-2"
        >
          <Users className="w-4 h-4" />
          {detectingCommunities ? t('knowledgeGraph.detecting', 'Detecting...') : t('knowledgeGraph.detectCommunities', 'Detect Communities')}
        </Button>
        
        <ConfirmDeleteDialog onOk={handleDeleteKnowledgeGraph}>
          <Button
            variant="outline"
            size={'sm'}
            className="flex items-center gap-2"
          >
            <Trash2 className="w-4 h-4" />
            {t('common.delete')}
          </Button>
        </ConfirmDeleteDialog>
      </div>
      
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
