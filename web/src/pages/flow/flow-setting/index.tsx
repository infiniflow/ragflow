import { useTranslate } from '@/hooks/common-hooks';
import { Modal, Typography } from 'antd';
import React from 'react';
import styles from './index.less';
const { Paragraph } = Typography;
export function useFlowSettingModal() {
  const [visibleSettingModal, setVisibleSettingMModal] = React.useState(false);

  return {
    visibleSettingModal,
    setVisibleSettingMModal,
  };
}

type FlowSettingModalProps = {
  visible: boolean;
  hideModal: () => void;
  id: string;
};
export const FlowSettingModal = ({
  hideModal,
  id,
  visible,
}: FlowSettingModalProps) => {
  const { t } = useTranslate('flow');
  return (
    <Modal
      title={'Agent Setting'}
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
  );
};
