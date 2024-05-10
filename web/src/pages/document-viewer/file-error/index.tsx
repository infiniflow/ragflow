import { Alert, Flex } from 'antd';

import { useTranslate } from '@/hooks/commonHooks';
import styles from './index.less';

const FileError = () => {
  const { t } = useTranslate('fileManager');
  return (
    <Flex align="center" justify="center" className={styles.errorWrapper}>
      <Alert type="error" message={<h1>{t('fileError')}</h1>}></Alert>
    </Flex>
  );
};

export default FileError;
