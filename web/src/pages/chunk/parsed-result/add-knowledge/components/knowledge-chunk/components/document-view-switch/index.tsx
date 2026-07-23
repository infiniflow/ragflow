import DocumentPreview from '@/components/document-preview';
import DocumentHeader from '@/components/document-preview/document-header';
import { Segmented, type SegmentedValue } from '@/components/ui/segmented';
import Representation, {
  type ClickableNode,
} from '@/pages/chunk/representation';
import { File, LayoutList } from 'lucide-react';
import { useCallback, useState } from 'react';
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
  onChunkIdsChange?: (chunkIds: string[]) => void;
}

export default function DocumentViewSwitch({
  documentInfo,
  fileType,
  highlights,
  setWidthAndHeight,
  url,
  onChunkIdsChange,
}: DocumentViewSwitchProps) {
  const { t } = useTranslation();
  const [viewMode, setViewMode] = useState<ViewMode>(ViewMode.Preview);

  const handleNodeClick = useCallback(
    (node: ClickableNode) => {
      onChunkIdsChange?.(node.source_chunk_ids ?? []);
    },
    [onChunkIdsChange],
  );

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
          <span>{t('common.preview', 'Preview')}</span>
        </div>
      ),
    },
    {
      value: ViewMode.Representations,
      label: (
        <div className="flex items-center gap-1">
          <LayoutList className="h-4 w-4" />
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
          />
        ) : (
          <Representation onNodeClick={handleNodeClick} />
        )}
      </div>
    </>
  );
}
