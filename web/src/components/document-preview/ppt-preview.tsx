import message from '@/components/ui/message';
import request from '@/utils/request';
import classNames from 'classnames';
import { init } from 'pptx-preview';
import { useEffect, useRef } from 'react';
interface PptPreviewerProps {
  className?: string;
  url: string;
}

export const PptPreviewer: React.FC<PptPreviewerProps> = ({
  className,
  url,
}) => {
  // const url = useGetDocumentUrl();
  const wrapper = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const fetchDocument = async () => {
    const res = await request(url, {
      method: 'GET',
      responseType: 'blob',
      onError: () => {
        message.error('Document parsing failed');
        console.error('Error loading document:', url);
      },
    });
    console.log(res);
    try {
      const arrayBuffer = await res.data.arrayBuffer();

      if (containerRef.current) {
        let width = 500;
        let height = 900;
        if (containerRef.current) {
          width = containerRef.current.clientWidth - 50;
          height = containerRef.current.clientHeight - 50;
        }
        let pptxPrviewer = init(containerRef.current, {
          width: width,
          height: height,
        });
        pptxPrviewer.preview(arrayBuffer);
      }
    } catch (err) {
      message.error('ppt parse failed');
    }
  };

  useEffect(() => {
    if (url) {
      fetchDocument();
    }
  }, [url]);

  return (
    <div
      ref={containerRef}
      className={classNames(
        'relative w-full h-full p-4 bg-background-paper border border-border-normal rounded-md ppt-previewer',
        className,
      )}
    >
      <div className="overflow-auto p-2">
        <div className="flex flex-col gap-4">
          <div ref={wrapper} />
        </div>
      </div>
    </div>
  );
};
