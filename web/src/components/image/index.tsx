import { api_host } from '@/utils/api';
import classNames from 'classnames';
import React from 'react';
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover';

interface IImage extends React.ImgHTMLAttributes<HTMLImageElement> {
  id: string;
  t?: string | number;
}

const Image = ({ id, t, className, ...props }: IImage) => {
  return (
    <img
      {...props}
      src={`${api_host}/document/image/${id}${t ? `?_t=${t}` : ''}`}
      alt=""
      className={classNames('max-w-[45vw] max-h-[40wh] block', className)}
    />
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
