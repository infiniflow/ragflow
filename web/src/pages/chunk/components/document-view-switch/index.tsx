import DocumentPreview from '@/components/document-preview';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import Representation, {
  type ClaimsPanelState,
  type ClickableNode,
  type EvidencePanelState,
} from '@/pages/chunk/representation';
import { RepresentationSelect } from '@/pages/chunk/representation/components/representation-select';
import { useGraphEntitySearch } from '@/pages/chunk/representation/hooks/use-graph-entity-search';
import { File } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IHighlight } from 'react-pdf-highlighter';

enum ViewMode {
  Preview = 'preview',
  Representations = 'representations',
}

// The two view pills reuse the outline button's hover styling while their
// view is active. The select trigger carries `!bg-bg-input`, so the active
// background needs the important modifier to win there.
const ActivePillClass =
  'text-text-primary !bg-border-button border-border-default';

interface DocumentViewSwitchProps {
  fileType: string;
  highlights: IHighlight[];
  setWidthAndHeight: (width: number, height: number) => void;
  url: string;
  positions?: number[][];
  onChunkIdsChange?: (chunkIds: string[]) => void;
  // The artifact tree publishes its claims / evidence panels upward so the page
  // can render them as a resizable middle column instead of inside the tree.
  onClaimsPanelChange?: (panel: ClaimsPanelState | null) => void;
  onEvidencePanelChange?: (panel: EvidencePanelState | null) => void;
}

export default function DocumentViewSwitch({
  fileType,
  highlights,
  setWidthAndHeight,
  url,
  positions,
  onChunkIdsChange,
  onClaimsPanelChange,
  onEvidencePanelChange,
}: DocumentViewSwitchProps) {
  const { t } = useTranslation();
  const [viewMode, setViewMode] = useState<ViewMode>(ViewMode.Preview);

  const handleNodeClick = useCallback(
    (node: ClickableNode) => {
      onChunkIdsChange?.(node.source_chunk_ids ?? []);
    },
    [onChunkIdsChange],
  );

  // Owned here instead of inside the tree: the tab row's template dropdown
  // needs the template list even while the preview view is showing.
  const graphSearch = useGraphEntitySearch(handleNodeClick);
  const { templates, selectedTemplateId, handleTemplateChange } = graphSearch;

  const handlePreviewClick = useCallback(() => {
    setViewMode(ViewMode.Preview);
    if (viewMode !== ViewMode.Preview) {
      onChunkIdsChange?.([]);
    }
  }, [onChunkIdsChange, viewMode]);

  // The template dropdown doubles as the representations tab: picking a
  // template switches the left column to that representation.
  const handleTemplatePillChange = useCallback(
    (templateId: string) => {
      handleTemplateChange(templateId);
      setViewMode(ViewMode.Representations);
    },
    [handleTemplateChange],
  );

  return (
    <>
      {templates.length > 0 && (
        <div className="flex items-center gap-2 p-5 pb-0">
          <Button
            variant="outline"
            className={cn(
              'w-fit',
              viewMode === ViewMode.Preview && ActivePillClass,
            )}
            onClick={handlePreviewClick}
          >
            <File />
            {t('common.preview')}
          </Button>
          <RepresentationSelect
            templates={templates}
            value={selectedTemplateId}
            onChange={handleTemplatePillChange}
            triggerClassName={cn(
              viewMode === ViewMode.Representations && ActivePillClass,
            )}
          />
        </div>
      )}

      <div className="flex-1 h-0 min-h-0 overflow-hidden p-5 pt-2.5 [&>section]:h-full [&>section]:min-h-0">
        {viewMode === ViewMode.Preview ? (
          <DocumentPreview
            className="h-full min-h-0 overflow-auto [&_img]:max-w-full [&_img]:h-auto"
            fileType={fileType}
            highlights={highlights}
            setWidthAndHeight={setWidthAndHeight}
            url={url}
            positions={positions}
          />
        ) : (
          <Representation
            graphSearch={graphSearch}
            onClaimsPanelChange={onClaimsPanelChange}
            onEvidencePanelChange={onEvidencePanelChange}
          />
        )}
      </div>
    </>
  );
}
