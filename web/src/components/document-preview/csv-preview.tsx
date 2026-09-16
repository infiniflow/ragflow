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
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';

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
  const tableRef = useRef<HTMLDivElement>(null);
  const [contentWidth, setContentWidth] = useState<number>(0);

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

  // Measure the natural width of the table (header + data determine column
  // widths together via table layout) so the container can be sized to match.
  // This ensures horizontal scroll reaches the last column.
  useEffect(() => {
    const el = tableRef.current;
    if (!el) return;
    const update = () => setContentWidth(el.offsetWidth);
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
      className={classNames(
        'relative w-full h-full bg-background-paper border border-border-normal rounded-md',
        'overflow-auto',
        className,
      )}
      onScroll={handleScroll}
    >
      {isLoading ? (
        <div className="absolute inset-0 flex items-center justify-center">
          <Spin />
        </div>
      ) : data ? (
        /* Use CSS table layout so header and data cells share consistent
           column widths. The table's natural width determines the scroll
           range, ensuring all columns are reachable via horizontal scroll. */
        <div
          style={{ width: contentWidth || undefined }}
          className={contentWidth ? '' : 'w-fit'}
        >
          <div ref={tableRef} className="table w-fit">
            {/* Sticky header row */}
            <div className="table-row-group">
              <div className="table-row sticky top-0 z-10 bg-background-header-bar">
                {data.headers.map((header, index) => (
                  <div
                    key={`header-${index}`}
                    className="table-cell px-6 py-3 text-left text-sm font-medium text-text-primary whitespace-nowrap border-b border-border-normal"
                  >
                    {header}
                  </div>
                ))}
              </div>
            </div>
            {/* Data rows with virtual scroll padding */}
            <div className="table-row-group">
              {/* Top spacer row */}
              {startIdx > 0 && (
                <div className="table-row" style={{ height: startIdx * rowHeight }}>
                  <div className="table-cell border-b border-border-normal" />
                </div>
              )}
              {data.rows.slice(startIdx, endIdx).map((row, i) => {
                const actualIndex = startIdx + i;
                return (
                  <div key={`row-${actualIndex}`} className="table-row hover:bg-gray-50">
                    {row.map((cell, cellIndex) => (
                      <div
                        key={`cell-${actualIndex}-${cellIndex}`}
                        className="table-cell px-6 py-2 whitespace-nowrap text-sm text-text-secondary border-b border-border-normal overflow-hidden text-ellipsis"
                        style={{ height: rowHeight }}
                      >
                        {cell || '-'}
                      </div>
                    ))}
                  </div>
                );
              })}
              {/* Bottom spacer row */}
              {data.rows.length - endIdx > 0 && (
                <div className="table-row" style={{ height: (data.rows.length - endIdx) * rowHeight }}>
                  <div className="table-cell border-b border-border-normal" />
                </div>
              )}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
};

export default CSVFileViewer;
