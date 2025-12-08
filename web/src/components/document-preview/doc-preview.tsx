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

// Word document preview component. Behavior:
// 1) Fetches the document as a Blob.
// 2) Detects .docx input via a ZIP header probe.
// 3) Renders .docx using Mammoth; presents a controlled "unsupported" notice for non-ZIP payloads.
export const DocPreviewer: React.FC<DocPreviewerProps> = ({
  className,
  url,
}) => {
  const [htmlContent, setHtmlContent] = useState<string>('');
  const [loading, setLoading] = useState(false);

  // Determines whether the Blob represents a .docx document by checking for the ZIP
  // file signature ("PK") in the initial bytes. A valid .docx file is a ZIP container
  // and always begins with:
  //     50 4B 03 04  ("PK..")
  //
  // Legacy .doc files use the CFBF binary format, commonly starting with:
  //     D0 CF 11 E0 A1 B1 1A E1
  //
  // Note that some files distributed with a “.doc” extension may internally be .docx
  // documents (e.g., renamed files or files produced by systems that export .docx
  // content under a .doc filename). These files will still present the ZIP signature
  // and are therefore treated as supported .docx payloads. The header inspection
  // ensures correct routing regardless of filename or reported extension.
  const isZipLikeBlob = async (blob: Blob): Promise<boolean> => {
    try {
      const headerSlice = blob.slice(0, 4);
      const buf = await headerSlice.arrayBuffer();
      const bytes = new Uint8Array(buf);

      // ZIP files start with "PK" (0x50, 0x4B)
      return bytes.length >= 2 && bytes[0] === 0x50 && bytes[1] === 0x4b;
    } catch (e) {
      console.error('Failed to inspect blob header', e);
      return false;
    }
  };

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

      // Execution path selection: ZIP-like payloads are treated as .docx and rendered via Mammoth;
      // non-ZIP payloads receive an explicit unsupported notice.
      const looksLikeZip = await isZipLikeBlob(blob);

      if (!looksLikeZip) {
        // Non-ZIP payload (likely legacy .doc or another format): skip Mammoth processing.
        setHtmlContent(`
          <div class="flex h-full items-center justify-center">
            <div class="border border-dashed border-border-normal rounded-xl p-8 max-w-2xl text-center">
              <p class="text-2xl font-bold mb-4">
                Preview is not available for this Word document
              </p>
              <p class="italic text-sm text-muted-foreground leading-relaxed">
                Mammoth supports modern <code>.docx</code> files only.<br/>
                The file header does not indicate a <code>.docx</code> ZIP archive.
              </p>
            </div>
          </div>
        `);
        return;
      }

      // ZIP-like payload: parse as .docx with Mammoth
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
      message.error('Failed to parse document.');
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
