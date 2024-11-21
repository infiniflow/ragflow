import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { Modal, Typography } from 'antd';

import { useParams } from 'umi';
import styles from './index.less';

const { Paragraph, Link } = Typography;

const FlowIdModal = ({ hideModal }: IModalProps<any>) => {
  const { t } = useTranslate('flow');
  const { id } = useParams();

  return (
    <Modal
      title={'Agent ID'}
      open
      onCancel={hideModal}
      cancelButtonProps={{ style: { display: 'none' } }}
      onOk={hideModal}
      okText={t('close', { keyPrefix: 'common' })}
    >
      <Paragraph copyable={{ text: id }} className={styles.id}>
        {id}
      </Paragraph>
      <Link
        href="https://ragflow.io/docs/dev/http_api_reference#create-session-with-an-agent"
        target="_blank"
      >
        {t('howUseId')}
      </Link>
    </Modal>
  );
};

export default FlowIdModal;
