import { Alert, Flex } from 'antd';

import { useTranslate } from '@/hooks/common-hooks';
import React from 'react';
import styles from './index.less';

const FileError = ({ children }: React.PropsWithChildren) => {
  const { t } = useTranslate('fileManager');
  return (
    <Flex align="center" justify="center" className={styles.errorWrapper}>
      <Alert
        type="error"
        message={<h2>{children || t('fileError')}</h2>}
      ></Alert>
    </Flex>
  );
};

export default FileError;
