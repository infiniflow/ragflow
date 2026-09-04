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

import ePub, { Book, NavItem, Rendition } from 'epubjs';
import { useCallback, useEffect, useRef, useState } from 'react';
import { normalizeEpubEncoding } from '@/utils/epub-util';

export interface EpubTocItem {
  href: string;
  label: string;
}

const flattenToc = (items: NavItem[]): EpubTocItem[] =>
  items.flatMap(({ href, label, subitems }) => [
    { href, label },
    ...(subitems ? flattenToc(subitems) : []),
  ]);

// The epub iframe is a separate document and does not inherit the page's
// CSS variables, so resolve the theme tokens and inject them as literal
// colors into the book's default theme.
const applyContentTheme = (rendition: Rendition, isDark: boolean) => {
  const styles = getComputedStyle(document.documentElement);
  const textPrimary = styles.getPropertyValue('--text-primary').trim();
  const bgBase = styles.getPropertyValue('--bg-base').trim();
  rendition.themes.default({
    body: {
      color: textPrimary ? `rgb(${textPrimary})` : isDark ? '#fff' : '#000',
      background: bgBase || (isDark ? '#161618' : '#fff'),
    },
  });
};

export const useEpubBook = (blob: Blob | null, isDark: boolean) => {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const renditionRef = useRef<Rendition | null>(null);
  const bookRef = useRef<Book | null>(null);
  const [toc, setToc] = useState<EpubTocItem[]>([]);
  const [rendering, setRendering] = useState(false);
  const [parseError, setParseError] = useState(false);
  // Render reads the flag asynchronously; a ref avoids re-opening the
  // whole book just because the theme flipped.
  const isDarkRef = useRef(isDark);
  isDarkRef.current = isDark;

  useEffect(() => {
    const container = containerRef.current;
    if (!blob || !container) {
      setToc([]);
      return;
    }
    let cancelled = false;
    // rendition.manager only exists after display() has started; resizing
    // before that throws inside epubjs. Gate the observer on first render.
    let ready = false;
    let lastSize = { width: 0, height: 0 };

    setRendering(true);
    setParseError(false);
    setToc([]);

    const render = async () => {
      try {
        // Pass binary data, not a `blob:` URL string: epubjs classifies URL
        // strings by file extension, so a `blob:` URL (no extension) would be
        // treated as a directory, making it fetch META-INF/container.xml over
        // HTTP and silently fail to open.
        const data = await blob.arrayBuffer();
        const book = ePub(await normalizeEpubEncoding(data));
        bookRef.current = book;
        // Book.open failures are only emitted as an event (book.opened never
        // rejects), so without this listener display() would hang forever.
        book.on('openFailed', (err: Error) => {
          if (!cancelled) {
            setParseError(true);
            setRendering(false);
            console.error('Error opening EPUB file:', err);
          }
        });
        const rendition = book.renderTo(container, {
          width: '100%',
          height: '100%',
          flow: 'paginated',
        });
        renditionRef.current = rendition;
        await rendition.display();
        applyContentTheme(rendition, isDarkRef.current);
        const navigation = await book.loaded.navigation;
        ready = true;
        if (!cancelled) setToc(flattenToc(navigation.toc));
      } catch (err) {
        if (!cancelled) {
          setParseError(true);
          console.error('Error rendering EPUB file:', err);
        }
      } finally {
        if (!cancelled) setRendering(false);
      }
    };

    render();

    // epubjs paginated flow splits the content into columns sized at
    // render time; when the container changes size the columns must be
    // re-measured via rendition.resize or pages break mid-text.
    const observer = new ResizeObserver(() => {
      const width = container.clientWidth;
      const height = container.clientHeight;
      if (!ready || (width === lastSize.width && height === lastSize.height)) {
        return;
      }
      lastSize = { width, height };
      if (renditionRef.current && width > 0 && height > 0) {
        renditionRef.current.resize(width, height);
      }
    });
    observer.observe(container);

    return () => {
      cancelled = true;
      observer.disconnect();
      renditionRef.current?.destroy();
      renditionRef.current = null;
      bookRef.current?.destroy();
      bookRef.current = null;
    };
  }, [blob]);

  // Re-theme the already-rendered book when the app theme changes.
  useEffect(() => {
    if (renditionRef.current) {
      applyContentTheme(renditionRef.current, isDark);
    }
  }, [isDark]);

  const goToHref = useCallback((href: string) => {
    renditionRef.current?.display(href);
  }, []);
  const nextPage = useCallback(() => renditionRef.current?.next(), []);
  const prevPage = useCallback(() => renditionRef.current?.prev(), []);

  return {
    containerRef,
    toc,
    rendering,
    parseError,
    goToHref,
    nextPage,
    prevPage,
  };
};
