import { IModalProps } from '@/interfaces/common';
import { Drawer } from 'antd';

const FlowDrawer = ({ visible, hideModal }: IModalProps<any>) => {
  return (
    <Drawer
      title="Basic Drawer"
      placement="right"
      //   closable={false}
      onClose={hideModal}
      open={visible}
      getContainer={false}
      mask={false}
    >
      <p>Some contents...</p>
    </Drawer>
  );
};

export default FlowDrawer;
