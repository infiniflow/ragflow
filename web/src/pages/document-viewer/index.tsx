import { Images } from '@/constants/common';
import { api_host } from '@/utils/api';
import { Flex, Image } from 'antd';
import { useParams, useSearchParams } from 'umi';
import Docx from './docx';
import Excel from './excel';
import Pdf from './pdf';

import styles from './index.less';

// TODO: The interface returns an incorrect content-type for the SVG.

const DocumentViewer = () => {
  const { id: documentId } = useParams();
  const api = `${api_host}/file/get/${documentId}`;
  const [currentQueryParameters] = useSearchParams();
  const ext = currentQueryParameters.get('ext');

  return (
    <section className={styles.viewerWrapper}>
      {Images.includes(ext!) && (
        <Flex className={styles.image} align="center" justify="center">
          <Image src={api} preview={false}></Image>
        </Flex>
      )}
      {ext === 'pdf' && <Pdf url={api}></Pdf>}
      {(ext === 'xlsx' || ext === 'xls') && <Excel filePath={api}></Excel>}

      {ext === 'docx' && <Docx filePath={api}></Docx>}
    </section>
  );
};

export default DocumentViewer;
