import { Authorization } from '@/constants/authorization';
import { restAPIv1 } from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
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

// Check if a URL requires authentication (internal API URLs)
// Only attach Authorization headers to same-origin requests to prevent token leakage
const isAuthRequiredUrl = (url: string): boolean => {
  try {
    const parsedUrl = new URL(url, window.location.origin);
    if (parsedUrl.origin !== window.location.origin) {
      return false;
    }
    return (
      parsedUrl.pathname.startsWith('/api/v1/') ||
      parsedUrl.pathname.includes('/documents/images/')
    );
  } catch {
    return false;
  }
};

export const useDocumentImageUrl = (id: string, t?: string | number) => {
  const directUrl = useMemo(() => buildDocumentImageUrl(id, t), [id, t]);
  const [imageUrl, setImageUrl] = useState<string>('');

  useEffect(() => {
    // For non-API URLs (e.g., base64, external URLs), use directly
    if (!isAuthRequiredUrl(directUrl)) {
      setImageUrl(directUrl);
      return;
    }

    // For API URLs that require authentication, always fetch with auth headers
    const authorization = getAuthorization();
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

/**
 * Hook to convert any authenticated URL to a blob URL for use in <img> tags.
 * Use this for thumbnail URLs or any other API URLs that require authentication.
 */
export const useAuthenticatedImageUrl = (url: string | undefined | null) => {
  const [imageUrl, setImageUrl] = useState<string>('');

  useEffect(() => {
    if (!url || !isAuthRequiredUrl(url)) {
      setImageUrl(url || '');
      return;
    }

    const authorization = getAuthorization();
    let cancelled = false;
    setImageUrl('');

    const { promise, release } = fetchDocumentImage(url, authorization);
    promise
      .then((blobUrl) => {
        if (!cancelled) {
          setImageUrl(blobUrl);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setImageUrl('');
        }
      });

    return () => {
      cancelled = true;
      release();
    };
  }, [url]);

  return imageUrl;
};

/**
 * Component that renders an <img> tag with proper authentication for API URLs.
 * Use this instead of <img src={apiUrl}> when the URL requires authentication.
 */
export const AuthenticatedImg = ({
  src,
  alt,
  className,
  fallback,
  ...props
}: React.ImgHTMLAttributes<HTMLImageElement> & {
  fallback?: React.ReactNode;
}) => {
  const authenticatedSrc = useAuthenticatedImageUrl(src);

  if (!authenticatedSrc) return fallback ?? null;

  return (
    <img
      src={authenticatedSrc}
      alt={alt}
      className={className}
      {...props}
    />
  );
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
