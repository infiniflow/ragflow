import CopyToClipboard from '@/components/copy-to-clipboard';
import { useSetModalState } from '@/hooks/common-hooks';
import { IRemoveMessageById } from '@/hooks/logic-hooks';
import {
  DeleteOutlined,
  DislikeOutlined,
  LikeOutlined,
  SoundOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { Radio } from 'antd';
import { useCallback } from 'react';
import SvgIcon from '../svg-icon';
import FeedbackModal from './feedback-modal';
import { useRemoveMessage, useSendFeedback } from './hooks';
import PromptModal from './prompt-modal';

interface IProps {
  messageId: string;
  content: string;
  prompt?: string;
}

export const AssistantGroupButton = ({
  messageId,
  content,
  prompt,
}: IProps) => {
  const { visible, hideModal, showModal, onFeedbackOk, loading } =
    useSendFeedback(messageId);
  const {
    visible: promptVisible,
    hideModal: hidePromptModal,
    showModal: showPromptModal,
  } = useSetModalState();

  const handleLike = useCallback(() => {
    onFeedbackOk({ thumbup: true });
  }, [onFeedbackOk]);

  return (
    <>
      <Radio.Group size="small">
        <Radio.Button value="a">
          <CopyToClipboard text={content}></CopyToClipboard>
        </Radio.Button>
        <Radio.Button value="b">
          <SoundOutlined />
        </Radio.Button>
        <Radio.Button value="c" onClick={handleLike}>
          <LikeOutlined />
        </Radio.Button>
        <Radio.Button value="d" onClick={showModal}>
          <DislikeOutlined />
        </Radio.Button>
        {prompt && (
          <Radio.Button value="e" onClick={showPromptModal}>
            <SvgIcon name={`prompt`} width={16}></SvgIcon>
          </Radio.Button>
        )}
      </Radio.Group>
      {visible && (
        <FeedbackModal
          visible={visible}
          hideModal={hideModal}
          onOk={onFeedbackOk}
          loading={loading}
        ></FeedbackModal>
      )}
      {promptVisible && (
        <PromptModal
          visible={promptVisible}
          hideModal={hidePromptModal}
          prompt={prompt}
        ></PromptModal>
      )}
    </>
  );
};

interface UserGroupButtonProps extends IRemoveMessageById {
  messageId: string;
  content: string;
}

export const UserGroupButton = ({
  content,
  messageId,
  removeMessageById,
}: UserGroupButtonProps) => {
  const { onRemoveMessage, loading } = useRemoveMessage(
    messageId,
    removeMessageById,
  );
  return (
    <Radio.Group size="small">
      <Radio.Button value="a">
        <CopyToClipboard text={content}></CopyToClipboard>
      </Radio.Button>
      <Radio.Button value="b">
        <SyncOutlined />
      </Radio.Button>
      <Radio.Button value="c" onClick={onRemoveMessage} disabled={loading}>
        <DeleteOutlined spin={loading} />
      </Radio.Button>
    </Radio.Group>
  );
};
