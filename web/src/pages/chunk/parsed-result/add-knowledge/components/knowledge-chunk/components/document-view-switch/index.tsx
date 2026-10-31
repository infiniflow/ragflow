import DocumentPreview from '@/components/document-preview';
import DocumentHeader from '@/components/document-preview/document-header';
import { Segmented, type SegmentedValue } from '@/components/ui/segmented';
import Representation, {
  type ClaimsPanelState,
  type ClickableNode,
  type EvidencePanelState,
} from '@/pages/chunk/representation';
import type { OntologyDetailPanelState } from '@/components/structure-graph/ontology-model-graph/detail-panel';
import { File, LayoutList } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IHighlight } from 'react-pdf-highlighter';

enum ViewMode {
  Preview = 'preview',
  Representations = 'representations',
}

interface DocumentViewSwitchProps {
  documentInfo?: {
    size: number;
    name: string;
    create_date: string;
  };
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
  // Same contract for the ontology graph's class panel: it is a panel like any
  // other, so the page's own divider must own it.
  onOntologyPanelChange?: (panel: OntologyDetailPanelState | null) => void;
  // Forwarded for the same reason: the page reclaims the chunk column's width
  // while the ontology representation is on screen, and only the tree view knows
  // which template is selected.
  onOntologyViewChange?: (active: boolean) => void;
}

export default function DocumentViewSwitch({
  documentInfo,
  fileType,
  highlights,
  setWidthAndHeight,
  url,
  positions,
  onChunkIdsChange,
  onClaimsPanelChange,
  onEvidencePanelChange,
  onOntologyPanelChange,
  onOntologyViewChange,
}: DocumentViewSwitchProps) {
  const { t } = useTranslation();
  const [viewMode, setViewMode] = useState<ViewMode>(ViewMode.Preview);

  const handleNodeClick = useCallback(
    (node: ClickableNode) => {
      onChunkIdsChange?.(node.source_chunk_ids ?? []);
    },
    [onChunkIdsChange],
  );

  // Retract the ontology claim as soon as this component stops showing the
  // representations: the preview unmounts `Representation`, so its own cleanup
  // is the only other chance to retract, and a state that merely *depends on an
  // unmount* is a state that can be left behind. The switch is where the mode is
  // decided, so the switch is where "the ontology is no longer on screen" is
  // stated.
  useEffect(() => {
    if (viewMode === ViewMode.Representations) return;
    onOntologyViewChange?.(false);
  }, [viewMode, onOntologyViewChange]);

  const handleViewModeChange = useCallback(
    (value: SegmentedValue) => {
      setViewMode(value as ViewMode);
      if (value === ViewMode.Preview && viewMode !== ViewMode.Preview) {
        onChunkIdsChange?.([]);
      }
    },
    [onChunkIdsChange, viewMode],
  );

  const options = [
    {
      value: ViewMode.Preview,
      label: (
        <div className="flex items-center gap-1">
          <File className="h-4 w-4" />
          <span>{t('common.preview')}</span>
        </div>
      ),
    },
    {
      value: ViewMode.Representations,
      label: (
        <div className="flex items-center gap-1">
          <LayoutList className="h-4 w-4" />
          {/* Product term: kept as-is in every locale, like RAPTOR. */}
          <span>Artifact</span>
        </div>
      ),
    },
  ];

  return (
    <>
      <DocumentHeader
        className="flex-1 min-w-0"
        wrapperClassName="flex items-center justify-between p-5 pb-0 gap-2"
        size={documentInfo?.size ?? 0}
        name={documentInfo?.name ?? ''}
        create_date={documentInfo?.create_date ?? ''}
      >
        <Segmented
          options={options}
          value={viewMode}
          onChange={handleViewModeChange}
        />
      </DocumentHeader>

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
            onNodeClick={handleNodeClick}
            onClaimsPanelChange={onClaimsPanelChange}
            onEvidencePanelChange={onEvidencePanelChange}
            onOntologyPanelChange={onOntologyPanelChange}
            onOntologyViewChange={onOntologyViewChange}
          />
        )}
      </div>
    </>
  );
}
