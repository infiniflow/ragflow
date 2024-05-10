import { ExceptiveType, Images } from '@/constants/common';
import { api_host } from '@/utils/api';
import { Flex, Image } from 'antd';
import FileViewer from 'react-file-viewer';
import { useParams, useSearchParams } from 'umi';
import Excel from './excel';
import Pdf from './pdf';

import styles from './index.less';

// TODO: The interface returns an incorrect content-type for the SVG.

const isNotExceptiveType = (ext: string) => ExceptiveType.indexOf(ext) === -1;

const DocumentViewer = () => {
  const { id: documentId } = useParams();
  const api = `${api_host}/file/get/${documentId}`;
  const [currentQueryParameters] = useSearchParams();
  const ext = currentQueryParameters.get('ext');

  const onError = (e: any) => {
    console.error(e, 'error in file-viewer');
  };

  return (
    <section className={styles.viewerWrapper}>
      {Images.includes(ext!) && (
        <Flex className={styles.image} align="center" justify="center">
          <Image src={api} preview={false}></Image>
        </Flex>
      )}
      {ext === 'pdf' && <Pdf url={api}></Pdf>}
      {(ext === 'xlsx' || ext === 'xls') && <Excel filePath={api}></Excel>}
      {isNotExceptiveType(ext!) && (
        <FileViewer fileType={ext} filePath={api} onError={onError} />
      )}
    </section>
  );
};

export default DocumentViewer;
