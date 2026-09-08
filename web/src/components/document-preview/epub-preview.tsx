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
import { useIsDarkTheme } from '@/components/theme-provider';
import classNames from 'classnames';
import { ChevronLeft, ChevronRight, List } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useEpubBook } from './use-epub-book';

type EpubPreviewerProps = { className?: string; url: string };

export const EpubPreviewer = ({ className, url }: EpubPreviewerProps) => {
  const isDark = useIsDarkTheme();
  const [blob, setBlob] = useState<Blob | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState(false);
  const {
    containerRef,
    toc,
    rendering,
    parseError,
    goToHref,
    nextPage,
    prevPage,
  } = useEpubBook(blob, isDark);
  const [tocOpen, setTocOpen] = useState(false);

  useEffect(() => {
    if (!url) {
      setBlob(null);
      return;
    }
    let cancelled = false;

    setLoading(true);
    setLoadError(false);
    setBlob(null);

    request(url, {
      method: 'GET',
      responseType: 'blob',
      onError: (err: unknown) => {
        message.error('Failed to load file');
        setLoadError(true);
        console.error('Error loading EPUB file:', err);
      },
    })
      .then((res) => {
        if (!cancelled) setBlob(res.data);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [url]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'ArrowLeft') {
        prevPage();
      } else if (e.key === 'ArrowRight') {
        nextPage();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [prevPage, nextPage]);

  return (
    <div
      className={classNames(
        'relative w-full h-full flex bg-bg-base border border-border-normal rounded-md overflow-hidden',
        className,
      )}
    >
      {tocOpen && (
        <aside className="w-60 shrink-0 h-full overflow-auto border-r border-border-normal bg-bg-card p-2">
          <nav aria-label="Table of contents">
            {toc.map((item) => (
              <button
                key={item.href}
                onClick={() => goToHref(item.href)}
                className="block w-full truncate rounded px-2 py-1.5 text-left text-sm text-text-secondary hover:bg-bg-card hover:text-text-primary"
              >
                {item.label}
              </button>
            ))}
          </nav>
        </aside>
      )}

      <div className="relative flex-1 h-full min-w-0">
        <button
          aria-label={
            tocOpen ? 'Hide table of contents' : 'Show table of contents'
          }
          onClick={() => setTocOpen(!tocOpen)}
          className="absolute left-2 top-2 z-10 flex size-8 items-center justify-center rounded-md border border-border-normal bg-bg-component text-text-secondary hover:text-text-primary"
        >
          <List className="size-4"></List>
        </button>

        {/* epub page surface follows the app theme; content colors are
            injected from the same tokens in use-epub-book */}
        <div ref={containerRef} className="w-full h-full bg-bg-base"></div>

        <button
          aria-label="Previous page"
          onClick={prevPage}
          className="absolute bottom-4 left-4 z-10 flex size-8 items-center justify-center rounded-md border border-border-normal bg-bg-component text-text-secondary hover:text-text-primary"
        >
          <ChevronLeft className="size-4"></ChevronLeft>
        </button>
        <button
          aria-label="Next page"
          onClick={nextPage}
          className="absolute bottom-4 right-4 z-10 flex size-8 items-center justify-center rounded-md border border-border-normal bg-bg-component text-text-secondary hover:text-text-primary"
        >
          <ChevronRight className="size-4"></ChevronRight>
        </button>
      </div>

      {(loading || rendering) && (
        <div className="absolute inset-0 z-20 flex items-center justify-center bg-bg-base/80">
          <Spin />
        </div>
      )}

      {!loading && !rendering && (loadError || parseError) && (
        <div className="absolute inset-0 z-20 flex items-center justify-center text-text-secondary text-sm">
          Failed to load file
        </div>
      )}
    </div>
  );
};
