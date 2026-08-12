/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import message from '@/components/ui/message';
import { Spin } from '@/components/ui/spin';
import request from '@/utils/request';
import {
  DocxEditorViewer,
  packageToArrayBuffer,
  parseDocx,
  useDocxEditor,
  useDocxPageLayout,
} from '@extend-ai/react-docx';
import classNames from 'classnames';
import { ZoomIn, ZoomOut } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';

import {
  isZipLikeBlob,
  useDocumentResizeObserver,
  useDocxPreviewZoom,
} from './hooks';

interface DocPreviewerProps {
  className?: string;
  url: string;
}

// @extend-ai/react-docx renders paragraphs without explicit line spacing at
// 0.88x the font size, which makes CJK glyph lines overlap. Word renders such
// paragraphs with the font's natural line height (~1.3x for CJK fonts).
// Inject a docDefaults-level line spacing (1.3 lines) so paragraphs that do
// not define their own line spacing get a consistent, readable line pitch.
// Paragraphs or styles with explicit line spacing are left untouched.
const DEFAULT_LINE_SPACING_TWIPS = 312; // 1.3 lines (240 twips per line)
const DEFAULT_LINE_SPACING_TAG = `<w:spacing w:line="${DEFAULT_LINE_SPACING_TWIPS}" w:lineRule="auto"/>`;

const DEFAULT_LINE_SPACING_PPR_DEFAULT = `<w:pPrDefault><w:pPr>${DEFAULT_LINE_SPACING_TAG}</w:pPr></w:pPrDefault>`;

// Ensure the docDefaults section of word/styles.xml defines a default line
// spacing. Returns the original XML when a default already exists.
const ensureDefaultLineSpacing = (stylesXml: string): string => {
  const docDefaults = stylesXml.match(
    /<w:docDefaults\b[^>]*>[\s\S]*?<\/w:docDefaults>/i,
  )?.[0];
  if (!docDefaults) {
    const selfClosing = stylesXml.match(/<w:docDefaults\b[^>]*\/>/i)?.[0];
    if (selfClosing) {
      return stylesXml.replace(
        selfClosing,
        `<w:docDefaults>${DEFAULT_LINE_SPACING_PPR_DEFAULT}</w:docDefaults>`,
      );
    }
    const stylesOpen = stylesXml.match(/<w:styles\b[^>]*>/i)?.[0];
    if (!stylesOpen) return stylesXml;
    return stylesXml.replace(
      stylesOpen,
      `${stylesOpen}<w:docDefaults>${DEFAULT_LINE_SPACING_PPR_DEFAULT}</w:docDefaults>`,
    );
  }

  const replaceDocDefaults = (next: string) =>
    stylesXml.replace(docDefaults, next);

  const pprDefault = docDefaults.match(
    /<w:pPrDefault\b[^>]*>[\s\S]*?<\/w:pPrDefault>/i,
  )?.[0];
  if (!pprDefault) {
    const selfClosing = docDefaults.match(/<w:pPrDefault\b[^>]*\/>/i)?.[0];
    if (selfClosing) {
      return replaceDocDefaults(
        docDefaults.replace(selfClosing, DEFAULT_LINE_SPACING_PPR_DEFAULT),
      );
    }
    const docDefaultsOpen = docDefaults.match(/<w:docDefaults\b[^>]*>/i)?.[0];
    if (!docDefaultsOpen) return stylesXml;
    return replaceDocDefaults(
      docDefaults.replace(
        docDefaultsOpen,
        `${docDefaultsOpen}${DEFAULT_LINE_SPACING_PPR_DEFAULT}`,
      ),
    );
  }

  const replacePprDefault = (next: string) =>
    replaceDocDefaults(docDefaults.replace(pprDefault, next));

  const spacingTag = pprDefault.match(/<w:spacing\b[^>]*?\/?>/i)?.[0];
  if (spacingTag) {
    // A default line spacing already exists; respect the document.
    if (/\bw:line\s*=/i.test(spacingTag)) return stylesXml;
    let tag = spacingTag.replace(/\s*\/?>$/, '');
    tag += ` w:line="${DEFAULT_LINE_SPACING_TWIPS}"`;
    if (!/\bw:lineRule\s*=/i.test(spacingTag)) {
      tag += ' w:lineRule="auto"';
    }
    return replacePprDefault(pprDefault.replace(spacingTag, `${tag}/>`));
  }

  const pprTag = pprDefault.match(/<w:pPr\b[^>]*>/i)?.[0];
  if (pprTag) {
    if (pprTag.endsWith('/>')) {
      return replacePprDefault(
        pprDefault.replace(
          pprTag,
          `<w:pPr>${DEFAULT_LINE_SPACING_TAG}</w:pPr>`,
        ),
      );
    }
    return replacePprDefault(
      pprDefault.replace(pprTag, `${pprTag}${DEFAULT_LINE_SPACING_TAG}`),
    );
  }
  const pprDefaultOpen = pprDefault.match(/<w:pPrDefault\b[^>]*>/i)?.[0];
  if (!pprDefaultOpen) return stylesXml;
  return replacePprDefault(
    pprDefault.replace(
      pprDefaultOpen,
      `${pprDefaultOpen}<w:pPr>${DEFAULT_LINE_SPACING_TAG}</w:pPr>`,
    ),
  );
};

// Repack the docx with a default line spacing so the preview renders
// consistent line pitch. Falls back to the original blob on any failure.
const normalizeDocxLineSpacing = async (blob: Blob): Promise<Blob> => {
  try {
    const pkg = await parseDocx(await blob.arrayBuffer());
    const stylesPart = pkg.parts.get('word/styles.xml');
    if (!stylesPart) return blob;
    const patched = ensureDefaultLineSpacing(stylesPart.content);
    if (patched === stylesPart.content) return blob;
    stylesPart.content = patched;
    return new Blob([packageToArrayBuffer(pkg)], {
      type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    });
  } catch (error) {
    console.warn('Failed to normalize docx line spacing:', error);
    return blob;
  }
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
  const { layout } = useDocxPageLayout(editor);
  const { containerWidth, setContainerRef } = useDocumentResizeObserver();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const showContent = !loading && !error;
  const { zoomScale, minZoom, maxZoom, handleZoomIn, handleZoomOut } =
    useDocxPreviewZoom({
      url,
      totalPages,
      pageWidthPx: layout?.pageWidthPx,
      containerWidth,
      enabled: showContent,
    });
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

      const normalizedBlob = await normalizeDocxLineSpacing(blob);

      if (cancelledRef.current) return;

      const file = new File([normalizedBlob], 'document.docx', {
        type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      });

      await importDocxFile(file);

      if (!cancelledRef.current) {
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
            disabled={loading || !!error || zoomScale <= minZoom}
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
            disabled={loading || !!error || zoomScale >= maxZoom}
            className="p-1 rounded hover:bg-gray-100 disabled:opacity-30 transition-opacity"
            onClick={handleZoomIn}
            aria-label="Zoom in"
          >
            <ZoomIn className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Viewer / Error area */}
      <div
        ref={setContainerRef}
        className="relative flex-1 overflow-auto bg-background-paper"
      >
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
          <div className="flex p-4" style={{ justifyContent: 'safe center' }}>
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
