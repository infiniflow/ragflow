import { getExtension } from '@/utils/document-util';
import SvgIcon from '../svg-icon';

import { useFetchDocumentThumbnailsByIds } from '@/hooks/use-document-request';
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

  useEffect(() => {
    if (id) {
      setDocumentIds([id]);
    }
  }, [id, setDocumentIds]);

  return fileThumbnail ? (
    <img src={fileThumbnail} className={styles.thumbnailImg}></img>
  ) : (
    <SvgIcon name={`file-icon/${fileExtension}`} width={24}></SvgIcon>
  );
};

export default FileIcon;
