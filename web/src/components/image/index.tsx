import { restAPIv1 } from '@/utils/api';
import request from '@/utils/request';
import classNames from 'classnames';
import React, { useEffect, useState } from 'react';
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover';

interface IImage extends React.ImgHTMLAttributes<HTMLImageElement> {
  id: string;
  t?: string | number;
  label?: string;
  onImageSrcChange?: (src: string) => void;
}

const Image = ({
  id,
  t,
  label,
  className,
  onImageSrcChange,
  ...props
}: IImage) => {
  const [imageSrc, setImageSrc] = useState('');

  useEffect(() => {
    let active = true;
    let objectUrl = '';
    setImageSrc('');
    onImageSrcChange?.('');

    const loadImage = async () => {
      try {
        const response = await request(
          `${restAPIv1}/documents/images/${id}${t ? `?_t=${t}` : ''}`,
          { method: 'GET', responseType: 'blob' },
        );
        objectUrl = URL.createObjectURL(response.data);
        if (active) {
          setImageSrc(objectUrl);
          onImageSrcChange?.(objectUrl);
        } else {
          URL.revokeObjectURL(objectUrl);
        }
      } catch {
        if (active) {
          setImageSrc('');
          onImageSrcChange?.('');
        }
      }
    };

    loadImage();
    return () => {
      active = false;
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl);
      }
    };
  }, [id, onImageSrcChange, t]);

  const imageElement = (
    <img
      {...props}
      src={imageSrc || undefined}
      className={classNames('max-w-[45vw] max-h-[40wh] block', className)}
    />
  );

  if (!label) {
    return imageElement;
  }

  return (
    <div className="relative inline-block w-full">
      {imageElement}
      <div className="absolute bottom-2 right-2 bg-accent-primary text-white px-2 py-0.5 rounded-xl text-xs font-normal backdrop-blur-sm">
        {label}
      </div>
    </div>
  );
};

export default Image;

export const ImageWithPopover = ({ id }: { id: string }) => {
  return (
    <Popover>
      <PopoverTrigger>
        <Image id={id} className="max-h-[100px] inline-block"></Image>
      </PopoverTrigger>
      <PopoverContent>
        <Image id={id} className="max-w-[100px] object-contain"></Image>
      </PopoverContent>
    </Popover>
  );
};
