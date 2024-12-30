import { PlusOutlined } from '@ant-design/icons';
import { CSSProperties } from 'react';

export const HandleIcon = () => {
  return (
    <PlusOutlined
      style={{ fontSize: 6, color: 'white', position: 'absolute', zIndex: 10 }}
    />
  );
};

export const RightHandleStyle: CSSProperties = {
  right: 0,
};

export const LeftHandleStyle: CSSProperties = {
  left: 0,
};

export default HandleIcon;
