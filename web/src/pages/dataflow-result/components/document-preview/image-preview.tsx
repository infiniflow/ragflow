import message from '@/components/ui/message';
import { Spin } from '@/components/ui/spin';
import request from '@/utils/request';
import classNames from 'classnames';
import { useEffect, useState } from 'react';

interface ImagePreviewerProps {
  className?: string;
  url: string;
}

export const ImagePreviewer: React.FC<ImagePreviewerProps> = ({
  className,
  url,
}) => {
  // const url = useGetDocumentUrl();
  const [imageSrc, setImageSrc] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);

  const fetchImage = async () => {
    setIsLoading(true);
    const res = await request(url, {
      method: 'GET',
      responseType: 'blob',
      onError: () => {
        message.error('Failed to load image');
        setIsLoading(false);
      },
    });
    const objectUrl = URL.createObjectURL(res.data);
    setImageSrc(objectUrl);
    setIsLoading(false);
  };
  useEffect(() => {
    if (url) {
      fetchImage();
    }
  }, [url]);

  useEffect(() => {
    return () => {
      if (imageSrc) {
        URL.revokeObjectURL(imageSrc);
      }
    };
  }, [imageSrc]);

  return (
    <div
      className={classNames(
        'relative w-full h-full p-4 bg-background-paper border border-border-normal rounded-md image-previewer',
        className,
      )}
    >
      {isLoading && (
        <div className="absolute inset-0 flex items-center justify-center">
          <Spin />
        </div>
      )}

      {!isLoading && imageSrc && (
        <div className="max-h-[80vh] overflow-auto p-2">
          <img
            src={imageSrc}
            alt={'image'}
            className="w-full h-auto max-w-full object-contain"
            onLoad={() => URL.revokeObjectURL(imageSrc!)}
          />
        </div>
      )}
    </div>
  );
};
