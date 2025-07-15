import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { useFetchKnowledgeGraph, useFetchKnowledgeBaseConfiguration, useResolveEntities, useDetectCommunities, useCheckDocumentParsing, useExtractEntities, useBuildGraph } from '@/hooks/knowledge-hooks';
import { Trash2, Network, Users, Zap, GitBranch, Settings } from 'lucide-react';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import ForceGraph from './force-graph';
import { useDeleteKnowledgeGraph } from './use-delete-graph';
import GraphRagConfiguration from './graph-rag-configuration';

const KnowledgeGraph: React.FC = () => {
  const { data } = useFetchKnowledgeGraph();
  const { data: knowledgeDetails } = useFetchKnowledgeBaseConfiguration();
  const { t } = useTranslation();
  const { handleDeleteKnowledgeGraph } = useDeleteKnowledgeGraph();
  const { resolveEntities, loading: resolvingEntities, progress: entityProgress, clearProgress: clearEntityProgress } = useResolveEntities();
  const { detectCommunities, loading: detectingCommunities, progress: communityProgress, clearProgress: clearCommunityProgress } = useDetectCommunities();
  const { extractEntities, loading: extractingEntities, progress: extractionProgress, clearProgress: clearExtractionProgress } = useExtractEntities();
  const { buildGraph, loading: buildingGraph, progress: buildProgress, clearProgress: clearBuildProgress } = useBuildGraph();
  const { isParsing } = useCheckDocumentParsing();
  const [showConfiguration, setShowConfiguration] = useState(false);

  const totalNodes = data?.graph?.total_nodes || 0;
  const totalEdges = data?.graph?.total_edges || 0;
  const displayedNodes = data?.graph?.nodes?.length || 0;
  const displayedEdges = data?.graph?.edges?.length || 0;
  
  // Calculate community count from graph data
  const communityCount = data?.graph?.nodes?.reduce((communities, node) => {
    if (node.communities && Array.isArray(node.communities)) {
      node.communities.forEach(community => communities.add(community));
    }
    return communities;
  }, new Set()).size || 0;

  // Workflow state logic for two-step process
  const hasDocuments = (knowledgeDetails?.chunk_num || 0) > 0;
  // Check if entity extraction completed with entities found
  const hasExtractedEntities = extractionProgress?.current_status === "completed" && (extractionProgress?.entities_found || 0) > 0;
  
  // Button enable/disable logic
  const canExtractEntities = !isParsing && hasDocuments && !extractingEntities;
  const canBuildGraph = !isParsing && hasDocuments && !buildingGraph;
  const canResolveEntities = !isParsing && hasDocuments && !resolvingEntities;
  const canDetectCommunities = !isParsing && hasDocuments && !detectingCommunities;
  const canDelete = (hasExtractedEntities || totalNodes > 0);

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

  const handleExtractEntities = async () => {
    try {
      await extractEntities();
    } catch (error) {
      console.error('Entity extraction failed:', error);
    }
  };

  const handleBuildGraph = async () => {
    try {
      await buildGraph();
    } catch (error) {
      console.error('Graph building failed:', error);
    }
  };

  return (
    <section className={'w-full h-full relative'}>
      {/* Action buttons */}
      <div className="absolute right-0 top-0 z-50 flex gap-2">
        <Button
          variant="outline"
          size={'sm'}
          onClick={() => setShowConfiguration(!showConfiguration)}
          className="flex items-center gap-2"
        >
          <Settings className="w-4 h-4" />
          {t('knowledgeGraph.configuration', 'Configuration')}
        </Button>
        <Button
          variant="outline"
          size={'sm'}
          onClick={handleExtractEntities}
          disabled={!canExtractEntities}
          className="flex items-center gap-2"
          title={isParsing ? t('knowledgeGraph.waitForParsing', 'Please wait for document parsing to complete') : 
                 totalNodes > 0 ? 'Re-extract entities from all documents (will update existing graph)' : 
                 'Extract entities from all documents in this knowledge base'}
        >
          <Zap className="w-4 h-4" />
          {extractingEntities ? t('knowledgeGraph.extracting', 'Extracting...') : t('knowledgeGraph.extractEntities', 'Extract Entities')}
        </Button>
        
        <Button
          variant="outline"
          size={'sm'}
          onClick={handleBuildGraph}
          disabled={!canBuildGraph}
          className="flex items-center gap-2"
        >
          <GitBranch className="w-4 h-4" />
          {buildingGraph ? t('knowledgeGraph.building', 'Building...') : t('knowledgeGraph.buildGraph', 'Build Graph')}
        </Button>
        
        <Button
          variant="outline"
          size={'sm'}
          onClick={handleResolveEntities}
          disabled={!canResolveEntities}
          className="flex items-center gap-2"
          title={isParsing ? t('knowledgeGraph.waitForParsing', 'Please wait for document parsing to complete') : undefined}
        >
          <Network className="w-4 h-4" />
          {resolvingEntities ? t('knowledgeGraph.resolving', 'Resolving...') : t('knowledgeGraph.resolveEntities', 'Resolve Entities')}
        </Button>
        
        <Button
          variant="outline"
          size={'sm'}
          onClick={handleDetectCommunities}
          disabled={!canDetectCommunities}
          className="flex items-center gap-2"
          title={isParsing ? t('knowledgeGraph.waitForParsing', 'Please wait for document parsing to complete') : undefined}
        >
          <Users className="w-4 h-4" />
          {detectingCommunities ? t('knowledgeGraph.detecting', 'Detecting...') : t('knowledgeGraph.detectCommunities', 'Detect Communities')}
        </Button>
        
        <ConfirmDeleteDialog onOk={handleDeleteKnowledgeGraph}>
          <Button
            variant="outline"
            size={'sm'}
            disabled={!canDelete}
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
          <div className="flex justify-between gap-4">
            <span>{t('knowledgeGraph.communities', 'Communities')}:</span>
            <span className="font-mono">
              {communityCount.toLocaleString()}
            </span>
          </div>
          {hasExtractedEntities && (
            <div className="flex justify-between gap-4">
              <span>{t('knowledgeGraph.entities', 'Entities')}:</span>
              <span className="font-mono">
                {(extractionProgress?.entities_found || 0).toLocaleString()}
              </span>
            </div>
          )}
          
          {/* Entity Extraction Progress */}
          {extractionProgress && (
            <div className="mt-3 pt-2 border-t border-gray-200">
              <div className="flex justify-between items-center mb-1">
                <div className="text-sm font-medium text-purple-700">
                  {t('knowledgeGraph.extractionProgress', 'Entity Extraction')}
                </div>
                {(extractionProgress.current_status === 'completed' || extractionProgress.current_status === 'error' || extractionProgress.current_status === 'failed') && (
                  <button
                    onClick={clearExtractionProgress}
                    className="text-gray-400 hover:text-gray-600 text-xs"
                    title="Dismiss"
                  >
                    ×
                  </button>
                )}
              </div>
              <div className="space-y-1">
                <div className="flex justify-between gap-2">
                  <span>{t('knowledgeGraph.documents', 'Documents')}:</span>
                  <span className="font-mono text-purple-600">
                    {extractionProgress.processed_documents}/{extractionProgress.total_documents}
                  </span>
                </div>
                <div className="flex justify-between gap-2">
                  <span>{t('knowledgeGraph.entitiesFound', 'Entities Found')}:</span>
                  <span className="font-mono text-green-600">
                    {extractionProgress.entities_found.toLocaleString()}
                  </span>
                </div>
                <div className="flex justify-between gap-2">
                  <span>{t('knowledgeGraph.status', 'Status')}:</span>
                  <span className="text-purple-600 capitalize">
                    {extractionProgress.current_status}
                  </span>
                </div>
              </div>
            </div>
          )}

          {/* Graph Building Progress */}
          {buildProgress && (
            <div className="mt-3 pt-2 border-t border-gray-200">
              <div className="flex justify-between items-center mb-1">
                <div className="text-sm font-medium text-indigo-700">
                  {t('knowledgeGraph.buildProgress', 'Graph Building')}
                </div>
                {(buildProgress.current_status === 'completed' || buildProgress.current_status === 'error' || buildProgress.current_status === 'failed') && (
                  <button
                    onClick={clearBuildProgress}
                    className="text-gray-400 hover:text-gray-600 text-xs"
                    title="Dismiss"
                  >
                    ×
                  </button>
                )}
              </div>
              <div className="space-y-1">
                <div className="flex justify-between gap-2">
                  <span>{t('knowledgeGraph.entities', 'Entities')}:</span>
                  <span className="font-mono text-indigo-600">
                    {buildProgress.processed_entities}/{buildProgress.total_entities}
                  </span>
                </div>
                <div className="flex justify-between gap-2">
                  <span>{t('knowledgeGraph.relationships', 'Relationships')}:</span>
                  <span className="font-mono text-green-600">
                    {buildProgress.relationships_created.toLocaleString()}
                  </span>
                </div>
                <div className="flex justify-between gap-2">
                  <span>{t('knowledgeGraph.status', 'Status')}:</span>
                  <span className="text-indigo-600 capitalize">
                    {buildProgress.current_status}
                  </span>
                </div>
              </div>
            </div>
          )}
          
          {/* Entity Resolution Progress */}
          {entityProgress && (
            <div className="mt-3 pt-2 border-t border-gray-200">
              <div className="flex justify-between items-center mb-1">
                <div className="text-sm font-medium text-orange-700">
                  {t('knowledgeGraph.entityProgress', 'Entity Resolution')}
                </div>
                {(entityProgress.current_status === 'completed' || entityProgress.current_status === 'error' || entityProgress.current_status === 'failed') && (
                  <button
                    onClick={clearEntityProgress}
                    className="text-gray-400 hover:text-gray-600 text-xs"
                    title="Dismiss"
                  >
                    ×
                  </button>
                )}
              </div>
              <div className="space-y-1">
                {entityProgress.total_pairs > 0 && (
                  <div className="flex justify-between gap-2">
                    <span>{t('knowledgeGraph.entityPairs', 'Entity Pairs')}:</span>
                    <span className="font-mono text-orange-600">
                      {entityProgress.processed_pairs}/{entityProgress.total_pairs}
                    </span>
                  </div>
                )}
                {entityProgress.remaining_pairs > 0 && (
                  <div className="flex justify-between gap-2">
                    <span>{t('knowledgeGraph.remaining', 'Remaining')}:</span>
                    <span className="font-mono text-gray-600">
                      {entityProgress.remaining_pairs.toLocaleString()}
                    </span>
                  </div>
                )}
                <div className="flex justify-between gap-2">
                  <span>{t('knowledgeGraph.status', 'Status')}:</span>
                  <span className="text-orange-600 capitalize">
                    {entityProgress.current_status}
                  </span>
                </div>
              </div>
            </div>
          )}
          
          {/* Community Detection Progress */}
          {communityProgress && (
            <div className="mt-3 pt-2 border-t border-gray-200">
              <div className="flex justify-between items-center mb-1">
                <div className="text-sm font-medium text-blue-700">
                  {t('knowledgeGraph.communityProgress', 'Community Detection')}
                </div>
                {(communityProgress.current_status === 'completed' || communityProgress.current_status === 'error' || communityProgress.current_status === 'failed') && (
                  <button
                    onClick={clearCommunityProgress}
                    className="text-gray-400 hover:text-gray-600 text-xs"
                    title="Dismiss"
                  >
                    ×
                  </button>
                )}
              </div>
              <div className="space-y-1">
                {communityProgress.total_communities > 0 && (
                  <div className="flex justify-between gap-2">
                    <span>{t('knowledgeGraph.communities', 'Communities')}:</span>
                    <span className="font-mono text-blue-600">
                      {communityProgress.processed_communities}/{communityProgress.total_communities}
                    </span>
                  </div>
                )}
                {communityProgress.tokens_used > 0 && (
                  <div className="flex justify-between gap-2">
                    <span>{t('knowledgeGraph.tokensUsed', 'Tokens Used')}:</span>
                    <span className="font-mono text-green-600">
                      {communityProgress.tokens_used.toLocaleString()}
                    </span>
                  </div>
                )}
                <div className="flex justify-between gap-2">
                  <span>{t('knowledgeGraph.status', 'Status')}:</span>
                  <span className="text-blue-600 capitalize">
                    {communityProgress.current_status}
                  </span>
                </div>
              </div>
            </div>
          )}
          
          {/* Document Parsing Status */}
          {isParsing && (
            <div className="mt-3 pt-2 border-t border-gray-200">
              <div className="text-sm font-medium text-yellow-700 mb-1">
                {t('knowledgeGraph.documentsParsing', 'Documents Parsing')}
              </div>
              <div className="text-xs text-yellow-600">
                {t('knowledgeGraph.waitForParsing', 'Please wait for document parsing to complete')}
              </div>
            </div>
          )}
        </div>
      </div>
      
      {/* Configuration Panel */}
      {showConfiguration && (
        <div className="absolute left-4 bottom-4 z-50 bg-white/95 backdrop-blur-sm rounded-lg p-4 shadow-lg border max-w-md">
          <div className="flex justify-between items-center mb-3">
            <h3 className="text-sm font-medium text-gray-700">
              {t('knowledgeGraph.graphRagConfig', 'GraphRAG Configuration')}
            </h3>
            <button
              onClick={() => setShowConfiguration(false)}
              className="text-gray-400 hover:text-gray-600"
            >
              ×
            </button>
          </div>
          <GraphRagConfiguration />
        </div>
      )}
      
      <ForceGraph data={data?.graph} show></ForceGraph>
    </section>
  );
};

export default KnowledgeGraph;
