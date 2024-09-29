import { Button, Card, Flex, Space, Typography } from 'antd';

import { useTranslate } from '@/hooks/common-hooks';
import styles from './index.less';

const { Paragraph } = Typography;

const BackendServiceApi = ({ show }: { show(): void }) => {
  const { t } = useTranslate('chat');

  return (
    <Card title={t('backendServiceApi')}>
      <Flex gap={8} vertical>
        {t('serviceApiEndpoint')}
        <Paragraph
          copyable={{ text: `${location.origin}/v1/api/` }}
          className={styles.linkText}
        >
          {location.origin}/v1/api/
        </Paragraph>
      </Flex>
      <Space size={'middle'}>
        <Button onClick={show}>{t('apiKey')}</Button>
        <a
          href={
            'https://github.com/infiniflow/ragflow/blob/main/docs/references/api.md'
          }
          target="_blank"
          rel="noreferrer"
        >
          <Button>{t('apiReference')}</Button>
        </a>
      </Space>
    </Card>
  );
};

export default BackendServiceApi;
