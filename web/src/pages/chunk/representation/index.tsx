import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { ExpandableSearchInput } from '@/components/expandable-search-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { SkeletonCard } from '@/components/skeleton-card';
import { Button } from '@/components/ui/button';
import {
  useDeleteDocumentStructureGraph,
  useFetchDocumentClaims,
} from '@/hooks/use-document-request';
import { Trash2 } from 'lucide-react';
import { memo, useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type ClickableNode,
  RepresentationRenderer,
} from '@/components/structure-graph/representation-renderer';
import { ClaimsPanel, NodeDetailPanel } from './components/claim-list';
import { RepresentationSelect } from './components/representation-select';
import { useGraphEntitySearch } from './hooks/use-graph-entity-search';

interface RepresentationProps {
  onNodeClick?: (node: ClickableNode) => void;
}

function Representation({ onNodeClick }: RepresentationProps) {
  const { t } = useTranslation();
  const { deleteDocumentStructureGraph, loading: deleting } =
    useDeleteDocumentStructureGraph();

  const [claimsLeaf, setClaimsLeaf] = useState<ClickableNode | null>(null);
  const [evidenceDetail, setEvidenceDetail] = useState<ClickableNode | null>(
    null,
  );

  const {
    data,
    loading,
    templates,
    selectedTemplateId,
    selectedTemplate,
    isGraphKind,
    entityOptions,
    searchKeyword,
    graphSelectValue,
    highlightNodeId,
    handleSelectEntity,
    handleNoMatchEnter,
    handleSearchKeywordChange,
    handleTemplateChange,
    handleNodeClick,
  } = useGraphEntitySearch(onNodeClick);

  // Tree leaves carry a claim-count badge: clicking one opens its claims here
  // in addition to the usual chunk navigation. Branch clicks close the panel —
  // they are pure structure and their descendants own the claims.
  const { data: claimsData, loading: claimsLoading } = useFetchDocumentClaims(
    claimsLeaf?.source_chunk_ids,
    selectedTemplateId,
  );

  const handleNodeClickWithClaims = useCallback(
    (node: ClickableNode) => {
      // Tree leaf cluster → its claims panel; a node carrying verified
      // evidence (page_index fact/conclusion) → its detail panel. Both also
      // forward to the usual chunk navigation.
      setClaimsLeaf(node.hasChildren === false ? node : null);
      setEvidenceDetail(node.evidence?.length ? node : null);
      handleNodeClick(node);
    },
    [handleNodeClick],
  );

  const handleDelete = useCallback(async () => {
    if (!selectedTemplateId) return;
    await deleteDocumentStructureGraph(selectedTemplateId);
  }, [deleteDocumentStructureGraph, selectedTemplateId]);

  return (
    <section className="p-5 rounded-2xl h-full flex flex-col">
      <div className="flex items-center justify-between">
        <RepresentationSelect
          templates={templates}
          value={selectedTemplateId}
          onChange={handleTemplateChange}
        />
        <div className="relative flex items-center gap-2">
          {isGraphKind ? (
            <SelectWithSearch
              options={entityOptions}
              value={graphSelectValue}
              onChange={handleSelectEntity}
              placeholder={t('knowledgeCompilation.searchEntity')}
              allowClear
              onNoMatchEnter={handleNoMatchEnter}
              disableAutoSelectOnEnter
            />
          ) : (
            <ExpandableSearchInput
              value={searchKeyword}
              onChange={handleSearchKeywordChange}
              placeholder={t('common.search')}
            />
          )}
          {templates.length > 0 && (
            <ConfirmDeleteDialog onOk={handleDelete}>
              <Button
                variant="ghost"
                size="icon"
                type="button"
                disabled={deleting}
                aria-label={t('common.delete', 'Delete')}
                className="absolute top-9 right-0"
              >
                <Trash2 className="h-5 w-5" />
              </Button>
            </ConfirmDeleteDialog>
          )}
        </div>
      </div>
      {loading && !data && <SkeletonCard className="mt-6" />}
      {!(loading && !data) && templates.length === 0 && (
        <div className="mt-6 text-text-secondary">
          {t('knowledgeCompilation.representationEmpty')}
        </div>
      )}
      {!(loading && !data) && templates.length > 0 && (
        <>
          <RepresentationRenderer
            template={selectedTemplate}
            onNodeClick={handleNodeClickWithClaims}
            highlightNodeId={highlightNodeId}
            totalEntities={data?.total_entities}
            returnedEntities={data?.returned_entities}
          />
          {claimsLeaf && (
            <ClaimsPanel
              clusterName={claimsLeaf.name}
              claims={claimsData?.claims ?? []}
              total={claimsData?.total ?? 0}
              loading={claimsLoading}
              onClose={() => setClaimsLeaf(null)}
            />
          )}
          {evidenceDetail && (
            <NodeDetailPanel
              nodeName={evidenceDetail.name}
              description={evidenceDetail.description}
              evidence={evidenceDetail.evidence ?? []}
              onClose={() => setEvidenceDetail(null)}
            />
          )}
        </>
      )}
    </section>
  );
}

export default memo(Representation);

export type { ClickableNode };
