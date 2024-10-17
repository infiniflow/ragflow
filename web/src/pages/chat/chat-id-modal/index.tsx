import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { Modal, Typography } from 'antd';

import styles from './index.less';

const { Paragraph } = Typography;

const ChatIdModal = ({
  visible,
  hideModal,
  id,
}: IModalProps<any> & { id: string; name?: string; idKey: string }) => {
  const { t } = useTranslate('chat');

  return (
    <>
      <Modal
        title={t('overview')}
        open={visible}
        onCancel={hideModal}
        cancelButtonProps={{ style: { display: 'none' } }}
        onOk={hideModal}
        okText={t('close', { keyPrefix: 'common' })}
      >
        <Paragraph copyable={{ text: id }} className={styles.id}>
          {id}
        </Paragraph>
      </Modal>
    </>
  );
};

export default ChatIdModal;
