import { IModalProps } from '@/interfaces/common';
import { Drawer } from 'antd';
import FlowChatBox from './box';

const ChatDrawer = ({ visible, hideModal }: IModalProps<any>) => {
  return (
    <Drawer
      title="Debug"
      placement="right"
      onClose={hideModal}
      open={visible}
      getContainer={false}
      width={window.innerWidth > 1278 ? '30%' : 470}
      mask={false}
      // zIndex={10000}
    >
      <FlowChatBox></FlowChatBox>
    </Drawer>
  );
};

export default ChatDrawer;
