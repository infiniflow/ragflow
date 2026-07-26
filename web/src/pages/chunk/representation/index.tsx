import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { ExpandableSearchInput } from '@/components/expandable-search-input';
import { Button } from '@/components/ui/button';
import {
  useDeleteDocumentStructureGraph,
  useFetchDocumentStructureGraph,
} from '@/hooks/use-document-request';
import { Trash2 } from 'lucide-react';
import { memo, useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type ClickableNode,
  RepresentationRenderer,
} from './components/representation-renderer';
import { RepresentationSelect } from './components/representation-select';
import { useSelectedTemplate } from './hooks/use-selected-template';

interface RepresentationProps {
  onNodeClick?: (node: ClickableNode) => void;
}

function Representation({ onNodeClick }: RepresentationProps) {
  const { t } = useTranslation();
  const { data, loading } = useFetchDocumentStructureGraph();
  const { deleteDocumentStructureGraph, loading: deleting } =
    useDeleteDocumentStructureGraph();
  const templates = data?.templates ?? [];
  const { selectedTemplateId, setSelectedTemplateId, selectedTemplate } =
    useSelectedTemplate(templates);
  const [searchKeyword, setSearchKeyword] = useState('');

  const handleSearchChange = useCallback((value: string) => {
    setSearchKeyword(value);
  }, []);

  const handleDelete = useCallback(async () => {
    if (!selectedTemplateId) return;
    await deleteDocumentStructureGraph(selectedTemplateId);
  }, [deleteDocumentStructureGraph, selectedTemplateId]);

  const handleNodeClick = useCallback(
    (node: ClickableNode) => {
      if (!node.source_chunk_ids?.length) return;
      onNodeClick?.(node);
    },
    [onNodeClick],
  );

  return (
    <section className="p-5 rounded-2xl h-full flex flex-col">
      <div className="flex items-center justify-between">
        <RepresentationSelect
          templates={templates}
          value={selectedTemplateId}
          onChange={setSelectedTemplateId}
        />
        <div className="relative flex items-center gap-2">
          <ExpandableSearchInput
            value={searchKeyword}
            onChange={handleSearchChange}
            placeholder={t('chunk.search', 'Search')}
          />
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
      {loading && (
        <div className="mt-6 text-text-secondary">
          {t('common.loading', 'Loading...')}
        </div>
      )}
      {!loading && templates.length === 0 && (
        <div className="mt-6 text-text-secondary">
          {t(
            'chunk.representationEmpty',
            'No representation templates available.',
          )}
        </div>
      )}
      {!loading && templates.length > 0 && (
        <RepresentationRenderer
          template={selectedTemplate}
          searchKeyword={searchKeyword}
          onNodeClick={handleNodeClick}
        />
      )}
    </section>
  );
}

export default memo(Representation);

export type { ClickableNode };
