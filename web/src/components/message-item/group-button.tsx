import CopyToClipboard from '@/components/copy-to-clipboard';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  DeleteOutlined,
  DislikeOutlined,
  LikeOutlined,
  SoundOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { Radio } from 'antd';
import FeedbackModal from './feedback-modal';

export const AssistantGroupButton = () => {
  const { visible, hideModal, showModal } = useSetModalState();

  return (
    <>
      <Radio.Group size="small">
        <Radio.Button value="a">
          <CopyToClipboard text="xxx"></CopyToClipboard>
        </Radio.Button>
        <Radio.Button value="b">
          <SoundOutlined />
        </Radio.Button>
        <Radio.Button value="c">
          <LikeOutlined />
        </Radio.Button>
        <Radio.Button value="d" onClick={showModal}>
          <DislikeOutlined />
        </Radio.Button>
      </Radio.Group>
      {visible && (
        <FeedbackModal visible={visible} hideModal={hideModal}></FeedbackModal>
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
