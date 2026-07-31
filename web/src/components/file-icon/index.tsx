import { getExtension } from '@/utils/document-util';
import SvgIcon from '../svg-icon';

import { useFetchDocumentThumbnailsByIds } from '@/hooks/use-document-request';
import { useAuthenticatedImageUrl } from '@/components/image';
import { useEffect } from 'react';
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
  const blobUrl = useAuthenticatedImageUrl(fileThumbnail);

  useEffect(() => {
    if (id) {
      setDocumentIds([id]);
    }
  }, [id, setDocumentIds]);

  return blobUrl ? (
    <img src={blobUrl} className={styles.thumbnailImg}></img>
  ) : (
    <SvgIcon name={`file-icon/${fileExtension}`} width={24}></SvgIcon>
  );
};

export default FileIcon;
