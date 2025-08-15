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
  // const url = useGetDocumentUrl();
  const [htmlContent, setHtmlContent] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const fetchDocument = async () => {
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
      const arrayBuffer = await res.data.arrayBuffer();
      const result = await mammoth.convertToHtml(
        { arrayBuffer },
        { includeDefaultStyleMap: true },
      );

      const styledContent = result.value
        .replace(/<p>/g, '<p class="mb-2">')
        .replace(/<h(\d)>/g, '<h$1 class="font-semibold mt-4 mb-2">');

      setHtmlContent(styledContent);
    } catch (err) {
      message.error('Document parsing failed');
      console.error('Error parsing document:', err);
    }
    setLoading(false);
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
