import { IModalProps } from '@/interfaces/common';
import { IFeedbackRequestBody } from '@/interfaces/request/chat';
import { Modal, Space } from 'antd';
import HightLightMarkdown from '../highlight-markdown';
import SvgIcon from '../svg-icon';

const PromptModal = ({
  visible,
  hideModal,
  prompt,
}: IModalProps<IFeedbackRequestBody> & { prompt?: string }) => {
  return (
    <Modal
      title={
        <Space>
          <SvgIcon name={`prompt`} width={18}></SvgIcon>
          Prompt
        </Space>
      }
      width={'80%'}
      open={visible}
      onCancel={hideModal}
      footer={null}
    >
      <HightLightMarkdown>{prompt}</HightLightMarkdown>
    </Modal>
  );
};

export default PromptModal;
