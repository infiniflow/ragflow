import { useFetchFlow } from '@/hooks/flow-hooks';
import { IModalProps } from '@/interfaces/common';
import { Drawer } from 'antd';
import { getDrawerWidth } from '../utils';
import FlowChatBox from './box';

const ChatDrawer = ({ visible, hideModal }: IModalProps<any>) => {
  const { data } = useFetchFlow();

  return (
    <Drawer
      title={data.title}
      placement="right"
      onClose={hideModal}
      open={visible}
      getContainer={false}
      width={getDrawerWidth()}
      mask={false}
      // zIndex={10000}
    >
      <FlowChatBox></FlowChatBox>
    </Drawer>
  );
};

export default ChatDrawer;
