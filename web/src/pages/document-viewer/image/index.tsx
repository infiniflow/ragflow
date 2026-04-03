import { Image as AntImage } from 'antd';
import { useEffect, useState } from 'react';

import { Authorization } from '@/constants/authorization';
import { getAuthorization } from '@/utils/authorization-util';

interface ImageProps {
  src: string;
  preview?: boolean;
}

const Image = ({ src, preview = false }: ImageProps) => {
  const [imageSrc, setImageSrc] = useState<string>('');

  useEffect(() => {
    const loadImage = async () => {
      try {
        const response = await fetch(src, {
          headers: {
            [Authorization]: getAuthorization(),
          },
        });
        const blob = await response.blob();
        const objectUrl = URL.createObjectURL(blob);
        setImageSrc(objectUrl);
      } catch (error) {
        console.error('Failed to load image:', error);
      }
    };

    loadImage();

    return () => {
      if (imageSrc) {
        URL.revokeObjectURL(imageSrc);
      }
    };
  }, [src]);

  return imageSrc ? <AntImage src={imageSrc} preview={preview} /> : null;
};

export default Image;
