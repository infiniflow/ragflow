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
import { decodeBlobText } from '@/utils/file-util';
import classNames from 'classnames';
import Papa from 'papaparse';
import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

interface CSVData {
  rows: string[][];
  headers: string[];
}

interface FileViewerProps {
  className?: string;
  url: string;
}

const CSVFileViewer: React.FC<FileViewerProps> = ({ className, url }) => {
  const [data, setData] = useState<CSVData | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const containerRef = useRef<HTMLDivElement>(null);
  const measureRef = useRef<HTMLDivElement>(null);
  const [naturalWidth, setNaturalWidth] = useState(0);

  /**
   * Parse CSV text into headers and data rows using PapaParse.
   * @param csvText - Raw CSV string to parse
   * @returns Parsed CSVData with headers array and rows 2D array
   */
  const parseCSV = (csvText: string): CSVData => {
    const result = Papa.parse<string[]>(csvText, {
      header: false,
      skipEmptyLines: false,
    });

    const rows = result.data as string[][];

    const headers = rows[0];
    const dataRows = rows.slice(1);

    return { headers, rows: dataRows };
  };

  useEffect(() => {
    const loadCSV = async () => {
      try {
        const res = await request(url, {
          method: 'GET',
          responseType: 'blob',
          onError: () => {
            message.error('file load failed');
            setIsLoading(false);
          },
        });

        // Handles UTF-8/UTF-16 (BOM) as well as GB2312/GBK files
        const csvText = await decodeBlobText(res.data);
        setData(parseCSV(csvText));
      } catch (error) {
        message.error('CSV file parse failed');
        console.error('Error loading CSV file:', error);
      } finally {
        setIsLoading(false);
      }
    };

    loadCSV();

    return () => {
      setData(null);
    };
  }, [url]);

  // Virtual row rendering state
  const [scrollTop, setScrollTop] = useState(0);
  const [containerHeight, setContainerHeight] = useState(0);
  const rowHeight = 36;

  // The true column count of the table: ragged rows may have more or fewer
  // cells than the header, so the "last column" is data-driven, not the
  // header length.
  const maxCols = useMemo(
    () =>
      data
        ? Math.max(data.headers.length, ...data.rows.map((row) => row.length))
        : 0,
    [data],
  );

  // Approximate rendered text width per glyph: fullwidth glyphs (CJK, kana,
  // etc.) are ~1em in the font, narrow ones (latin, digits) ~0.55em. Used to
  // pick the widest-looking cell per column for the measurement table below.
  const glyphScore = useCallback((cell: string) => {
    let width = 0;
    for (const ch of cell) {
      width += (ch.codePointAt(0) ?? 0) > 0x2e7f ? 1 : 0.55;
    }
    return width;
  }, []);

  // Widest cell per column (by glyph width), consumed by the hidden
  // measurement table below. Only rows in the virtualized window are ever
  // rendered, so the measurement table (always rendered) is what keeps the
  // wrapper's min-width independent of the current scroll position.
  const longestCells = useMemo(() => {
    if (!data) return [];
    const longest: string[] = [];
    const bestScores: number[] = [];
    const consider = (cell: string | undefined, index: number) => {
      if (!cell) return;
      const score = glyphScore(cell);
      if (bestScores[index] === undefined || score > bestScores[index]) {
        longest[index] = cell;
        bestScores[index] = score;
      }
    };
    data.headers.forEach((header, index) => consider(header, index));
    data.rows.forEach((row) => {
      for (let index = 0; index < row.length; index++) {
        consider(row[index], index);
      }
    });
    return longest;
  }, [data, glyphScore]);

  // Recalculate container height on resize and track scroll position
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const update = () => setContainerHeight(el.clientHeight);
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Measure the table's natural width from the hidden measurement table,
  // which always renders the widest cell of every column regardless of the
  // virtualized row window. A definite pixel min-width (instead of
  // min-w-max) keeps the width stable while scrolling and never triggers the
  // max-content/percentage-column circular sizing browsers can blow up on.
  useEffect(() => {
    const el = measureRef.current;
    if (!el) return;
    const update = () => setNaturalWidth(el.offsetWidth);
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, [data]);

  // Scroll handler on the actual scrolling container
  const handleScroll = useCallback((e: React.UIEvent<HTMLDivElement>) => {
    setScrollTop(e.currentTarget.scrollTop);
  }, []);

  // Calculate visible row range with overscan
  const { startIdx, endIdx } = useMemo(() => {
    const overscan = 10;
    const start = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);
    const visibleCount = Math.ceil(containerHeight / rowHeight) + 2 * overscan;
    const end = data ? Math.min(data.rows.length, start + visibleCount) : 0;
    return { startIdx: start, endIdx: end };
  }, [scrollTop, containerHeight, rowHeight, data]);

  return (
    <div
      ref={containerRef}
      className={classNames('relative w-full h-full overflow-auto', className)}
      onScroll={handleScroll}
    >
      {isLoading ? (
        <div className="absolute inset-0 flex items-center justify-center">
          <Spin />
        </div>
      ) : data ? (
        /* Border lives on the inner wrapper rather than the scroll
           container. When the CSV has only a few rows, the bordered box
           wraps just the rendered table instead of enclosing a tall empty
           area that visually reads as "truncated". naturalWidth (measured
           from the hidden measurement table below) keeps the wrapper as
           wide as the table's natural width so horizontal scroll still
           reaches the last column; when the table is narrower than the
           container it fills the container, and the last column
           (width: 100% in table auto layout) absorbs the leftover width. */
        <div
          style={{ minWidth: naturalWidth || undefined }}
          className="w-full bg-background-paper border border-border-normal rounded-md"
        >
          <div className="table w-full">
            {/* Sticky header row — padded with empty cells up to maxCols so
                the stretch column exists even where ragged data rows are
                shorter than the table's true last column. Ragged rows are
                padded with empty cells below so their bottom borders reach
                the true last column too. */}
            <div className="table-row-group">
              <div className="table-row sticky top-0 z-10 bg-bg-canvas">
                {Array.from({ length: maxCols }, (_, index) => {
                  const isPadded = index >= data.headers.length;
                  return (
                    <div
                      key={`header-${index}`}
                      className={classNames(
                        'table-cell px-6 py-3 text-left text-sm whitespace-nowrap border-b border-border-normal',
                        !isPadded && 'font-medium text-text-primary',
                        index === maxCols - 1 && 'w-full',
                      )}
                    >
                      {isPadded ? null : data.headers[index]}
                    </div>
                  );
                })}
              </div>
            </div>
            {/* Data rows with virtual scroll padding */}
            <div className="table-row-group">
              {/* Top spacer row */}
              {startIdx > 0 && (
                <div
                  className="table-row"
                  style={{ height: startIdx * rowHeight }}
                >
                  <div className="table-cell border-b border-border-normal" style={{ width: naturalWidth || undefined }} />
                </div>
              )}
              {data.rows.slice(startIdx, endIdx).map((row, i) => {
                const actualIndex = startIdx + i;
                return (
                  <div
                    key={`row-${actualIndex}`}
                    className="table-row hover:bg-gray-50"
                  >
                    {row.map((cell, cellIndex) => (
                      <div
                        key={`cell-${actualIndex}-${cellIndex}`}
                        className={classNames(
                          'table-cell px-6 py-2 whitespace-nowrap text-sm text-text-secondary overflow-hidden text-ellipsis border-b border-border-normal',
                          cellIndex === maxCols - 1 && 'w-full',
                        )}
                        style={{ height: rowHeight }}
                      >
                        {cell || '-'}
                      </div>
                    ))}
                    {/* Empty cells to complete ragged rows so the row's full-width
                        bottom border is drawn by the last padding cell instead
                        of disappearing through gaps between shorter rows. */}
                    {Array.from({ length: maxCols - row.length }, (_, padIdx) => (
                      <div
                        key={`pad-${actualIndex}-${padIdx}`}
                        className={classNames(
                          'table-cell border-b border-border-normal',
                          padIdx === maxCols - row.length - 1 && 'w-full',
                        )}
                        style={{ height: rowHeight }}
                      >
                        {''}
                      </div>
                    ))}
                  </div>
                );
              })}
              {/* Bottom spacer row */}
              {data.rows.length - endIdx > 0 && (
                <div
                  className="table-row"
                  style={{ height: (data.rows.length - endIdx) * rowHeight }}
                >
                  <div className="table-cell border-b border-border-normal" style={{ width: naturalWidth || undefined }} />
                </div>
              )}
            </div>
          </div>
          {/* Absolutely-positioned measurement table: always renders the widest
              cell of every column (regardless of the virtualized row
              window) and its offsetWidth is read back into naturalWidth.
              Invisible and out of flow so it never paints or scrolls. */}
          <div
            ref={measureRef}
            className="absolute top-0 left-0 invisible pointer-events-none"
            aria-hidden="true"
          >
            <div className="table w-fit">
              <div className="table-row">
                {longestCells.map((cell, index) => (
                  <div
                    key={`measure-${index}`}
                    className="table-cell px-6 text-sm whitespace-nowrap"
                  >
                    {cell}
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
};

export default CSVFileViewer;
