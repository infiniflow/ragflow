import { Button, Card, Flex, Space, Typography } from 'antd';

import { useTranslate } from '@/hooks/common-hooks';
import styles from './index.less';

const { Paragraph } = Typography;

const BackendServiceApi = ({ show }: { show(): void }) => {
  const { t } = useTranslate('chat');

  return (
    <Card
      title={
        <Space size={'large'}>
          <span>RAGFlow API</span>
          <Button onClick={show} type="primary">
            {t('apiKey')}
          </Button>
        </Space>
      }
    >
      <Flex gap={8} align="center">
        <b>{t('backendServiceApi')}</b>
        <Paragraph
          copyable={{ text: `${location.origin}` }}
          className={styles.apiLinkText}
        >
          {location.origin}
        </Paragraph>
      </Flex>
    </Card>
  );
};

export default BackendServiceApi;
