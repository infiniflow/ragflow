import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { ExpandableSearchInput } from '@/components/expandable-search-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { SkeletonCard } from '@/components/skeleton-card';
import { Button } from '@/components/ui/button';
import { useDeleteDocumentStructureGraph } from '@/hooks/use-document-request';
import { Trash2 } from 'lucide-react';
import { memo, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type ClickableNode,
  RepresentationRenderer,
} from '@/components/structure-graph/representation-renderer';
import { RepresentationSelect } from './components/representation-select';
import { useGraphEntitySearch } from './hooks/use-graph-entity-search';

interface RepresentationProps {
  onNodeClick?: (node: ClickableNode) => void;
}

function Representation({ onNodeClick }: RepresentationProps) {
  const { t } = useTranslation();
  const { deleteDocumentStructureGraph, loading: deleting } =
    useDeleteDocumentStructureGraph();

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
        <RepresentationRenderer
          template={selectedTemplate}
          onNodeClick={handleNodeClick}
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
