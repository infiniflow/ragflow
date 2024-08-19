import { getExtension } from '@/utils/document-util';
import SvgIcon from '../svg-icon';

import { useSelectFileThumbnails } from '@/hooks/knowledge-hooks';
import styles from './index.less';

interface IProps {
  name: string;
  id: string;
}

const FileIcon = ({ name, id }: IProps) => {
  const fileExtension = getExtension(name);
  // TODO: replace this line with react query
  const fileThumbnails = useSelectFileThumbnails();
  const fileThumbnail = fileThumbnails[id];

  return fileThumbnail ? (
    <img src={fileThumbnail} className={styles.thumbnailImg}></img>
  ) : (
    <SvgIcon name={`file-icon/${fileExtension}`} width={24}></SvgIcon>
  );
};

export default FileIcon;
