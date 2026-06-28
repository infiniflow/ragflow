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

type ImageCacheItem = {
  count: number;
  objectUrl?: string;
  promise?: Promise<string>;
  timer?: ReturnType<typeof setTimeout>;
};

const imageCache = new Map<string, ImageCacheItem>();

export const buildDocumentImageUrl = (id: string, t?: string | number) => {
  const params = new URLSearchParams();

  if (t) {
    params.set('_t', String(t));
  }

  const query = params.toString();
  return `${restAPIv1}/documents/images/${id}${query ? `?${query}` : ''}`;
};

const fetchDocumentImage = (url: string, authorization: string) => {
  const cacheKey = `${authorization}:${url}`;
  let item = imageCache.get(cacheKey);

  if (!item) {
    item = { count: 0 };
    imageCache.set(cacheKey, item);
  }
  if (item.timer) {
    clearTimeout(item.timer);
    item.timer = undefined;
  }
  item.count += 1;

  if (!item.promise) {
    item.promise = fetch(url, { headers: { [Authorization]: authorization } })
      .then((response) => {
        if (!response.ok) {
          throw new Error(response.statusText);
        }
        return response.blob();
      })
      .then((blob) => {
        item.objectUrl = URL.createObjectURL(blob);
        return item.objectUrl;
      })
      .catch((error) => {
        imageCache.delete(cacheKey);
        throw error;
      });
  }

  return {
    promise: item.promise,
    release: () => {
      item.count -= 1;
      if (item.count <= 0) {
        item.timer = setTimeout(() => {
          if (item.count <= 0) {
            if (item.objectUrl) {
              URL.revokeObjectURL(item.objectUrl);
            }
            imageCache.delete(cacheKey);
          }
        }, 30000);
      }
    },
  };
};

export const useDocumentImageUrl = (id: string, t?: string | number) => {
  const directUrl = useMemo(() => buildDocumentImageUrl(id, t), [id, t]);
  const [imageUrl, setImageUrl] = useState(() =>
    getAuthorization() && getSearchValue('shared_id') ? '' : directUrl,
  );

  useEffect(() => {
    const authorization = getAuthorization();
    if (!authorization || !getSearchValue('shared_id')) {
      setImageUrl(directUrl);
      return;
    }

    let ignore = false;
    setImageUrl('');
    const { promise, release } = fetchDocumentImage(directUrl, authorization);
    promise
      .then((url) => {
        if (ignore) {
          return;
        }
        setImageUrl(url);
      })
      .catch(() => {
        if (!ignore) {
          setImageUrl('');
        }
      });

    return () => {
      ignore = true;
      release();
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
