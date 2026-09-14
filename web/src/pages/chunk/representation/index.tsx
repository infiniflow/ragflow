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
import { memo, useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type ClickableNode,
  RepresentationRenderer,
} from '@/components/structure-graph/representation-renderer';
import type {
  ClaimsPanelState,
  EvidencePanelState,
} from './components/claim-list';
import { RepresentationSelect } from './components/representation-select';
import { useGraphEntitySearch } from './hooks/use-graph-entity-search';

export type {
  ClaimsPanelState,
  EvidencePanelState,
} from './components/claim-list';

interface RepresentationProps {
  onNodeClick?: (node: ClickableNode) => void;
  // The claims / evidence panels belong to the artifact page's middle column,
  // not inside this tree view. The selection still lives here (it is driven by
  // node clicks), but the resolved content is published upward and the page
  // decides where to render it.
  onClaimsPanelChange?: (panel: ClaimsPanelState | null) => void;
  onEvidencePanelChange?: (panel: EvidencePanelState | null) => void;
}

function Representation({
  onNodeClick,
  onClaimsPanelChange,
  onEvidencePanelChange,
}: RepresentationProps) {
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

  // Tree leaves carry a claim-count badge: clicking one opens its claims in the
  // artifact page's middle column, in addition to the usual chunk navigation.
  // Branch clicks close the panel — they are pure structure and their
  // descendants own the claims. The panels themselves are rendered by the page
  // (as a resizable column), so only the resolved content is published upward.
  const { data: claimsData, loading: claimsLoading } = useFetchDocumentClaims(
    claimsLeaf?.source_chunk_ids,
    selectedTemplateId,
  );

  const handleCloseClaims = useCallback(() => setClaimsLeaf(null), []);
  const handleCloseEvidence = useCallback(() => setEvidenceDetail(null), []);

  // Wait for the fetch to settle before committing to either panel. While it
  // is in flight, publish the claims panel in its loading state so the middle
  // column is stable and honest — publishing the node-detail panel during the
  // load made it flash first and then be overwritten by the claims panel (or,
  // for a node without claims, it just stayed there looking unexplained). The
  // page stacks both panels in one slot, so the choice is exclusive — and it
  // can only be made once the claim count is known.
  const claimsSettled = Boolean(claimsLeaf) && !claimsLoading;
  const hasClaims = claimsSettled && (claimsData?.claims?.length ?? 0) > 0;

  useEffect(() => {
    if (claimsLeaf && !claimsSettled) {
      onClaimsPanelChange?.({
        clusterName: claimsLeaf.name,
        claims: [],
        total: 0,
        loading: true,
        onClose: handleCloseClaims,
      });
      return () => onClaimsPanelChange?.(null);
    }
    // No claims and no node detail: keep the claims panel open with its empty
    // state when the click actually addressed chunks (a real leaf), so the
    // column answers "why nothing" instead of silently closing. Branch nodes
    // without source_chunk_ids close it -- they are pure structure.
    const showEmptyClaims =
      claimsSettled &&
      !hasClaims &&
      !evidenceDetail &&
      Boolean(claimsLeaf?.source_chunk_ids?.length);
    onClaimsPanelChange?.(
      claimsLeaf && (hasClaims || showEmptyClaims)
        ? {
            clusterName: claimsLeaf.name,
            claims: claimsData?.claims ?? [],
            total: claimsData?.total ?? 0,
            loading: claimsLoading,
            onClose: handleCloseClaims,
          }
        : null,
    );
    // Clear on unmount too: switching the left view back to the document
    // preview unmounts the tree, and the page must drop the column with it.
    return () => onClaimsPanelChange?.(null);
  }, [
    claimsLeaf,
    claimsData,
    claimsLoading,
    claimsSettled,
    evidenceDetail,
    hasClaims,
    handleCloseClaims,
    onClaimsPanelChange,
  ]);

  useEffect(() => {
    onEvidencePanelChange?.(
      claimsSettled && !hasClaims && evidenceDetail
        ? {
            nodeName: evidenceDetail.name,
            description: evidenceDetail.description,
            evidence: evidenceDetail.evidence ?? [],
            onClose: handleCloseEvidence,
          }
        : null,
    );
    return () => onEvidencePanelChange?.(null);
  }, [
    claimsSettled,
    evidenceDetail,
    handleCloseEvidence,
    hasClaims,
    onEvidencePanelChange,
  ]);

  const handleNodeClickWithClaims = useCallback(
    (node: ClickableNode) => {
      // Any node can own claims — a page_index heading covers the chunks of its
      // whole section, so the claims belonging to it are the ones sourced from
      // those chunks. Nothing here keys off ``badge``: the structure compiler
      // never writes ``claim_count``, so gating on it kept the panel shut for
      // every node. Whether the panel actually opens is decided by the fetch
      // below, once we know the node has claims.
      setClaimsLeaf(node);
      // The node-detail panel is ONLY for nodes carrying gate-verified quotes
      // (page_index fact/conclusion rows). A description alone would make
      // every tree node fall back to it when no claims exist -- an
      // unexplained block of compiled summary text in the claims slot.
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
      <div className="flex items-center gap-2">
        <RepresentationSelect
          templates={templates}
          value={selectedTemplateId}
          onChange={handleTemplateChange}
        />
        <div className="min-w-0">
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
        </div>
        {templates.length > 0 && (
          <ConfirmDeleteDialog onOk={handleDelete}>
            <Button
              variant="ghost"
              size="icon"
              type="button"
              disabled={deleting}
              aria-label={t('common.delete', 'Delete')}
              className="ml-auto shrink-0"
            >
              <Trash2 className="h-5 w-5" />
            </Button>
          </ConfirmDeleteDialog>
        )}
      </div>
      {loading && !data && <SkeletonCard className="mt-6" />}
      {!(loading && !data) && templates.length === 0 && (
        <div className="mt-6 text-text-secondary">
          {t('knowledgeCompilation.representationEmpty')}
        </div>
      )}
      {!(loading && !data) && templates.length > 0 && (
        <RepresentationRenderer
          template={selectedTemplate}
          onNodeClick={handleNodeClickWithClaims}
          highlightNodeId={highlightNodeId}
          totalEntities={data?.total_entities}
          returnedEntities={data?.returned_entities}
        />
      )}
    </section>
  );
}

export default memo(Representation);

export type { ClickableNode };
