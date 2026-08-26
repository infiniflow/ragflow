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

import classNames from 'classnames';
import { useEffect, useMemo, useRef, useState } from 'react';

import { Authorization } from '@/constants/authorization';
import { getAuthorization } from '@/utils/authorization-util';

interface HtmlPreviewerProps {
  className?: string;
  url: string;
  /** [page, charStart, charEnd, ...] or highlight by content text */
  positions?: number[][];
  highlightText?: string;
}

export const HtmlPreviewer: React.FC<HtmlPreviewerProps> = ({
  className,
  url,
  positions,
  highlightText,
}) => {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [html, setHtml] = useState('');

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      const res = await fetch(url, {
        headers: { [Authorization]: getAuthorization() },
      });
      const text = await res.text();
      if (!cancelled) {
        setHtml(text);
      }
    };
    if (url) {
      load().catch(() => {
        if (!cancelled) setHtml('');
      });
    }
    return () => {
      cancelled = true;
    };
  }, [url]);

  const srcDoc = useMemo(() => {
    if (!html) return '';
    return html;
  }, [html]);

  useEffect(() => {
    const iframe = iframeRef.current;
    if (!iframe || !srcDoc) return;

    const onLoad = () => {
      const doc = iframe.contentDocument;
      if (!doc?.body) return;

      const needle =
        highlightText?.trim() ||
        (positions?.[0]
          ? doc.body.textContent?.slice(positions[0][1], positions[0][2])
          : '');
      if (!needle) return;

      const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_TEXT);
      let node: Node | null;
      while ((node = walker.nextNode())) {
        const value = node.nodeValue || '';
        const idx = value.indexOf(needle.slice(0, Math.min(needle.length, 80)));
        if (idx < 0) continue;
        const range = doc.createRange();
        range.setStart(node, idx);
        range.setEnd(
          node,
          Math.min(idx + needle.length, value.length),
        );
        const mark = doc.createElement('mark');
        mark.style.background = '#ffe58f';
        try {
          range.surroundContents(mark);
          mark.scrollIntoView({ block: 'center', behavior: 'smooth' });
        } catch {
          // ignore invalid range boundaries across elements
        }
        break;
      }
    };

    iframe.addEventListener('load', onLoad);
    return () => iframe.removeEventListener('load', onLoad);
  }, [srcDoc, positions, highlightText]);

  return (
    <iframe
      ref={iframeRef}
      title="html-preview"
      sandbox="allow-same-origin"
      srcDoc={srcDoc}
      className={classNames('w-full h-full border-0 bg-background-paper', className)}
    />
  );
};
