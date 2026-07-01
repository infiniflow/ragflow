import { getExtension } from '@/utils/document-util';
import SvgIcon from '../svg-icon';

import { useFetchDocumentThumbnailsByIds } from '@/hooks/use-document-request';
import { Authorization } from '@/constants/authorization';
import { getAuthorization } from '@/utils/authorization-util';
import { useEffect, useState } from 'react';
import styles from './index.module.less';

interface IProps {
  name: string;
  id: string;
}

const FileIcon = ({ name, id }: IProps) => {
  const fileExtension = getExtension(name);

  const { data: fileThumbnails, setDocumentIds } =
    useFetchDocumentThumbnailsByIds();
  const fileThumbnail = fileThumbnails[id];

  const [blobUrl, setBlobUrl] = useState<string>('');

  useEffect(() => {
    if (id) {
      setDocumentIds([id]);
    }
  }, [id, setDocumentIds]);

  // Fetch authenticated image URL and convert to blob URL for <img> tag
  useEffect(() => {
    if (!fileThumbnail || !fileThumbnail.startsWith('/api/v1/')) {
      setBlobUrl(fileThumbnail || '');
      return;
    }

    let cancelled = false;
    const authorization = getAuthorization();

    const fetchImage = async () => {
      try {
        const response = await fetch(fileThumbnail, {
          headers: { [Authorization]: authorization },
        });
        if (response.ok && !cancelled) {
          const blob = await response.blob();
          const url = URL.createObjectURL(blob);
          setBlobUrl(url);
          return () => URL.revokeObjectURL(url);
        }
      } catch {
        // Silently fail, will show fallback icon
      }
      if (!cancelled) {
        setBlobUrl('');
      }
    };

    fetchImage();
    return () => {
      cancelled = true;
    };
  }, [fileThumbnail]);

  return blobUrl ? (
    <img src={blobUrl} className={styles.thumbnailImg}></img>
  ) : (
    <SvgIcon name={`file-icon/${fileExtension}`} width={24}></SvgIcon>
  );
};

export default FileIcon;
