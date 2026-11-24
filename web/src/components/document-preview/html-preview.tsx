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

import { Spin } from '@/components/ui/spin';
import { cn } from '@/lib/utils';
import { getDocumentPreviewBlob } from '@/services/file-manager-service';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { decodeHtmlPreview } from './html-encoding';

const HtmlPreviewHeader = `<!doctype html>
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; base-uri 'none'; form-action 'none'">
<style>:where(html) { background: Canvas; color: CanvasText; }</style>
`;

interface HtmlPreviewerProps {
  className?: string;
  url: string;
}

type PreviewState =
  | { status: 'loading' }
  | { status: 'error' }
  | { status: 'ready'; objectUrl: string };

export function HtmlPreviewer({ className, url }: HtmlPreviewerProps) {
  const { t } = useTranslation();
  const [preview, setPreview] = useState<PreviewState>({ status: 'loading' });

  useEffect(() => {
    if (!url) return;

    const controller = new AbortController();
    let objectUrl: string | undefined;

    async function loadPreview() {
      try {
        const file = await getDocumentPreviewBlob(url, controller.signal);
        if (controller.signal.aborted) return;
        // The preview API can return a JSON error body with HTTP 200.
        if (!file.size || file.type.split(';', 1)[0].trim() !== 'text/html') {
          setPreview({ status: 'error' });
          return;
        }
        const html = await decodeHtmlPreview(file);
        if (controller.signal.aborted) return;
        // Normalize the bytes so the trusted policy and the document are
        // always interpreted using the same encoding inside the sandbox.
        objectUrl = URL.createObjectURL(
          new Blob([HtmlPreviewHeader, html], {
            type: 'text/html;charset=utf-8',
          }),
        );
        setPreview({ status: 'ready', objectUrl });
      } catch {
        if (!controller.signal.aborted) setPreview({ status: 'error' });
      }
    }

    void loadPreview();
    return () => {
      controller.abort();
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [url]);

  if (!url) return null;

  return (
    <div className={cn('h-full min-h-0 w-full', className)}>
      {preview.status === 'loading' && (
        <div
          className="flex h-full items-center justify-center"
          role="status"
          aria-label={t('fileManager.preview')}
        >
          <Spin />
        </div>
      )}
      {preview.status === 'error' && (
        <div
          className="flex h-full items-center justify-center p-4 text-center text-text-secondary"
          role="alert"
        >
          {t('fileManager.htmlPreviewError')}
        </div>
      )}
      {preview.status === 'ready' && (
        <iframe
          className="block h-full w-full border-0"
          referrerPolicy="no-referrer"
          sandbox=""
          src={preview.objectUrl}
          title={t('fileManager.preview')}
        />
      )}
    </div>
  );
}
