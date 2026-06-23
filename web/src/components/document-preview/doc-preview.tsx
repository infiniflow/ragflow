import message from '@/components/ui/message';
import { Spin } from '@/components/ui/spin';
import request from '@/utils/request';
import { DocxEditorViewer, useDocxEditor } from '@extend-ai/react-docx';
import classNames from 'classnames';
import { ZoomIn, ZoomOut } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';

interface DocPreviewerProps {
  className?: string;
  url: string;
}

// ZIP file header bytes "PK"
const ZIP_HEADER_0 = 0x50;
const ZIP_HEADER_1 = 0x4b;

const isZipLikeBlob = async (blob: Blob): Promise<boolean> => {
  try {
    const headerSlice = blob.slice(0, 4);
    const buf = await headerSlice.arrayBuffer();
    const bytes = new Uint8Array(buf);
    return (
      bytes.length >= 2 &&
      bytes[0] === ZIP_HEADER_0 &&
      bytes[1] === ZIP_HEADER_1
    );
  } catch (e) {
    console.error('Failed to inspect blob header', e);
    return false;
  }
};

const ZOOM_STEPS = [25, 50, 75, 100, 125, 150, 175, 200] as const;

const clampZoom = (scale: number, direction: 1 | -1): number => {
  let idx = ZOOM_STEPS.indexOf(scale as (typeof ZOOM_STEPS)[number]);
  if (idx < 0) {
    if (direction > 0) {
      idx = ZOOM_STEPS.findIndex((v) => v > scale);
    } else {
      for (let i = ZOOM_STEPS.length - 1; i >= 0; i--) {
        if (ZOOM_STEPS[i] < scale) {
          idx = i;
          break;
        }
      }
    }
  }
  idx = Math.max(
    0,
    Math.min(ZOOM_STEPS.length - 1, idx < 0 ? 0 : idx + direction),
  );
  return ZOOM_STEPS[idx] ?? scale;
};

// Word document preview component.
// Uses @extend-ai/react-docx for canvas-based page-level rendering.
// Falls back to an unsupported notice for legacy .doc (non-ZIP) payloads.
export const DocPreviewer: React.FC<DocPreviewerProps> = ({
  className,
  url,
}) => {
  const editor = useDocxEditor({ initialFileName: 'document.docx' });
  const { importDocxFile, status, totalPages } = editor;
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [zoomScale, setZoomScale] = useState(100);
  const cancelledRef = useRef(false);

  // Fetch the document blob and load it into the editor
  const fetchDocument = useCallback(async () => {
    if (!url) return;

    cancelledRef.current = false;
    setLoading(true);
    setError(null);

    let res;
    try {
      res = await request(url, {
        method: 'GET',
        responseType: 'blob',
        onError: () => {
          if (!cancelledRef.current) {
            message.error('Document parsing failed');
            console.error('Error loading document:', url);
          }
        },
      });
    } catch {
      if (!cancelledRef.current) {
        setError('Failed to fetch document.');
        setLoading(false);
      }
      return;
    }

    if (cancelledRef.current) return;

    try {
      const blob: Blob = res.data;
      const looksLikeZip = await isZipLikeBlob(blob);

      if (!looksLikeZip) {
        setError(
          'This file header does not indicate a .docx ZIP archive. Only .docx files are supported.',
        );
        setLoading(false);
        return;
      }

      const file = new File([blob], 'document.docx', {
        type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      });

      await importDocxFile(file);

      if (!cancelledRef.current) {
        setZoomScale(100);
        setLoading(false);
      }
    } catch (err) {
      if (!cancelledRef.current) {
        message.error('Failed to parse document.');
        console.error('Error parsing document:', err);
        setLoading(false);
      }
    }
  }, [url, importDocxFile]);

  useEffect(() => {
    fetchDocument();
    return () => {
      cancelledRef.current = true;
    };
  }, [fetchDocument]);

  // Monitor editor status for library-level errors
  useEffect(() => {
    if (status === 'Only .docx files are supported') {
      setError(status);
      setLoading(false);
    }
  }, [status]);

  const handleZoomIn = useCallback(() => {
    setZoomScale((s) => clampZoom(s, 1));
  }, []);

  const handleZoomOut = useCallback(() => {
    setZoomScale((s) => clampZoom(s, -1));
  }, []);

  const showContent = !loading && !error;
  const pageCount = showContent && totalPages > 0 ? totalPages : 0;

  return (
    <div
      className={classNames(
        'relative w-full h-full flex flex-col bg-background-paper border border-border-normal rounded-md overflow-hidden',
        className,
      )}
    >
      {/* Toolbar */}
      <div className="flex items-center justify-between shrink-0 px-4 py-2 border-b border-border-normal bg-background-paper">
        <span className="text-sm text-muted-foreground">
          {loading ? 'Loading...' : error ? '' : `Page ${pageCount || '-'}`}
        </span>
        <div className="flex items-center gap-1">
          <button
            type="button"
            disabled={loading || !!error || zoomScale <= ZOOM_STEPS[0]}
            className="p-1 rounded hover:bg-gray-100 disabled:opacity-30 transition-opacity"
            onClick={handleZoomOut}
            aria-label="Zoom out"
          >
            <ZoomOut className="w-4 h-4" />
          </button>
          <span className="text-sm w-12 text-center tabular-nums select-none">
            {zoomScale}%
          </span>
          <button
            type="button"
            disabled={
              loading ||
              !!error ||
              zoomScale >= ZOOM_STEPS[ZOOM_STEPS.length - 1]
            }
            className="p-1 rounded hover:bg-gray-100 disabled:opacity-30 transition-opacity"
            onClick={handleZoomIn}
            aria-label="Zoom in"
          >
            <ZoomIn className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Viewer / Error area */}
      <div className="relative flex-1 overflow-auto bg-background-paper">
        {loading && (
          <div className="absolute inset-0 flex items-center justify-center">
            <Spin />
          </div>
        )}

        {error && !loading && (
          <div className="flex items-center justify-center h-full p-8">
            <div className="border border-dashed border-border-normal rounded-xl p-8 max-w-2xl text-center">
              <p className="text-2xl font-bold mb-4">
                Preview is not available for this Word document
              </p>
              <p className="italic text-sm text-muted-foreground leading-relaxed">
                @extend-ai/react-docx supports modern <code>.docx</code> files
                only.
                <br />
                {error}
              </p>
            </div>
          </div>
        )}

        {showContent && (
          <div className="flex justify-center p-4">
            <div style={{ zoom: zoomScale / 100 }}>
              <DocxEditorViewer
                editor={editor}
                mode="read-only"
                loadingState={
                  <div className="flex items-center justify-center p-8">
                    <Spin />
                  </div>
                }
                pageGapBackgroundColor="#f5f5f5"
              />
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
