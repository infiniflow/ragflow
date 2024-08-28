import CopyToClipboard from '@/components/copy-to-clipboard';
import {
  DeleteOutlined,
  DislikeOutlined,
  LikeOutlined,
  SoundOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { Radio } from 'antd';
import { useCallback } from 'react';
import FeedbackModal from './feedback-modal';
import { useSendFeedback } from './hooks';

interface IProps {
  messageId: string;
  content: string;
}

export const AssistantGroupButton = ({ messageId, content }: IProps) => {
  const { visible, hideModal, showModal, onFeedbackOk, loading } =
    useSendFeedback(messageId);

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
      </Radio.Group>
      {visible && (
        <FeedbackModal
          visible={visible}
          hideModal={hideModal}
          onOk={onFeedbackOk}
          loading={loading}
        ></FeedbackModal>
      )}
    </>
  );
};

export const UserGroupButton = () => {
  return (
    <Radio.Group size="small">
      <Radio.Button value="a">
        <CopyToClipboard text="xxx"></CopyToClipboard>
      </Radio.Button>
      <Radio.Button value="b">
        <SyncOutlined />
      </Radio.Button>
      <Radio.Button value="c">
        <DeleteOutlined />
      </Radio.Button>
    </Radio.Group>
  );
};
