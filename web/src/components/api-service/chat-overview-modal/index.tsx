import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { Modal } from 'antd';
import ApiContent from './api-content';

const ChatOverviewModal = ({
  visible,
  hideModal,
  id,
  idKey,
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
        width={'100vw'}
        okText={t('close', { keyPrefix: 'common' })}
      >
        <ApiContent id={id} idKey={idKey}></ApiContent>
      </Modal>
    </>
  );
};

export default ChatOverviewModal;
