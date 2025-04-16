import React from 'react';
import { Modal, Button } from 'antd';
import { DeleteOutlined } from '@ant-design/icons';

interface DeleteConfirmModalProps {
  visible: boolean;
  onOk: () => void;
  onCancel: () => void;
}

const DeleteConfirmModal: React.FC<DeleteConfirmModalProps> = ({
  visible,
  onOk,
  onCancel,
}) => {
  return (
    <Modal
      title="确定删除吗?"
      open={visible}
      footer={null}
      onCancel={onCancel}
      centered
    >
      <div style={{ textAlign: 'center', padding: '20px 0' }}>
        <div style={{ marginBottom: '20px' }}>确定删除吗?</div>
        <div style={{ display: 'flex', justifyContent: 'center', gap: '10px' }}>
          <Button onClick={onCancel}>
            否
          </Button>
          <Button 
            type="primary" 
            danger 
            icon={<DeleteOutlined />}
            onClick={onOk}
          >
            是
          </Button>
        </div>
      </div>
    </Modal>
  );
};

export default DeleteConfirmModal;
