import message from '@/components/ui/message';
import { Spin } from '@/components/ui/spin';
import request from '@/utils/request';
import classNames from 'classnames';
import mammoth from 'mammoth';
import { useEffect, useState } from 'react';

interface DocPreviewerProps {
  className?: string;
  url: string;
}

export const DocPreviewer: React.FC<DocPreviewerProps> = ({
  className,
  url,
}) => {
  const [htmlContent, setHtmlContent] = useState<string>('');
  const [loading, setLoading] = useState(false);

  const fetchDocument = async () => {
    if (!url) return;

    setLoading(true);

    const res = await request(url, {
      method: 'GET',
      responseType: 'blob',
      onError: () => {
        message.error('Document parsing failed');
        console.error('Error loading document:', url);
      },
    });

    try {
      const blob: Blob = res.data;
      const contentType: string =
        blob.type || (res as any).headers?.['content-type'] || '';

      // ---- Detect legacy .doc via MIME or URL ----
      const cleanUrl = url.split(/[?#]/)[0].toLowerCase();
      const isDocMime = /application\/msword/i.test(contentType);
      const isLegacyDocByUrl =
        cleanUrl.endsWith('.doc') && !cleanUrl.endsWith('.docx');
      const isLegacyDoc = isDocMime || isLegacyDocByUrl;

      if (isLegacyDoc) {
        // Do not call mammoth and do not throw an error; instead, show a note in the preview area
        setHtmlContent(`
          <div class="flex h-full items-center justify-center">
            <div class="border border-dashed border-border-normal rounded-xl p-8 max-w-2xl text-center">
              <p class="text-2xl font-bold mb-4">
                Preview not available for .doc files
              </p>
              <p class="italic text-sm text-muted-foreground leading-relaxed">
                Mammoth does not support <code>.doc</code> documents.<br/>
                Inline preview is unavailable.
              </p>
            </div>
          </div>
        `);
        return;
      }

      // ---- Standard .docx preview path ----
      const arrayBuffer = await blob.arrayBuffer();
      const result = await mammoth.convertToHtml(
        { arrayBuffer },
        { includeDefaultStyleMap: true },
      );

      const styledContent = result.value
        .replace(/<p>/g, '<p class="mb-2">')
        .replace(/<h(\d)>/g, '<h$1 class="font-semibold mt-4 mb-2">');

      setHtmlContent(styledContent);
    } catch (err) {
      // Only errors from the mammoth conversion path should surface here
      message.error('Document parsing failed');
      console.error('Error parsing document:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (url) {
      fetchDocument();
    }
  }, [url]);

  return (
    <div
      className={classNames(
        'relative w-full h-full p-4 bg-background-paper border border-border-normal rounded-md',
        className,
      )}
    >
      {loading && (
        <div className="absolute inset-0 flex items-center justify-center">
          <Spin />
        </div>
      )}

      {!loading && <div dangerouslySetInnerHTML={{ __html: htmlContent }} />}
    </div>
  );
};
