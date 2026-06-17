import { Authorization } from '@/constants/authorization';
import { restAPIv1 } from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import { getSearchValue } from '@/utils/common-util';
import classNames from 'classnames';
import React, { useEffect, useMemo, useState } from 'react';
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover';

interface IImage extends React.ImgHTMLAttributes<HTMLImageElement> {
  id: string;
  t?: string | number;
  label?: string;
}

export const buildDocumentImageUrl = (id: string, t?: string | number) => {
  const params = new URLSearchParams();

  if (t) {
    params.set('_t', String(t));
  }

  const query = params.toString();
  return `${restAPIv1}/documents/images/${id}${query ? `?${query}` : ''}`;
};

export const useDocumentImageUrl = (id: string, t?: string | number) => {
  const directUrl = useMemo(() => buildDocumentImageUrl(id, t), [id, t]);
  const [imageUrl, setImageUrl] = useState(directUrl);

  useEffect(() => {
    const authorization = getAuthorization();
    if (!authorization || !getSearchValue('shared_id')) {
      setImageUrl(directUrl);
      return;
    }

    let objectUrl = '';
    let ignore = false;
    setImageUrl('');
    fetch(directUrl, { headers: { [Authorization]: authorization } })
      .then((response) => {
        if (!response.ok) {
          throw new Error(response.statusText);
        }
        return response.blob();
      })
      .then((blob) => {
        if (ignore) {
          return;
        }
        objectUrl = URL.createObjectURL(blob);
        setImageUrl(objectUrl);
      })
      .catch(() => {
        if (!ignore) {
          setImageUrl('');
        }
      });

    return () => {
      ignore = true;
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl);
      }
    };
  }, [directUrl]);

  return imageUrl;
};

const Image = ({ id, t, label, className, ...props }: IImage) => {
  const src = useDocumentImageUrl(id, t);
  const imageElement = (
    <img
      {...props}
      src={src || undefined}
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
